package k8s

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
)

type HTTPRoute struct {
	Name              string
	Entrypoint        string
	Host              string
	PathPrefix        string
	Method            string
	Headers           map[string]string
	Middlewares       []string
	MiddlewareConfigs []config.Middleware
	Service           string
}

type ServiceEndpoint struct {
	Service string
	URL     string
	Weight  int
}

type Source interface {
	ListRoutes(context.Context) ([]HTTPRoute, error)
	ListEndpoints(context.Context) ([]ServiceEndpoint, error)
	Watch(context.Context, func(), func(error)) (io.Closer, error)
}

type Provider struct {
	name    string
	source  Source
	options Options
	logger  *slog.Logger
}

type Options struct {
	DefaultEntrypointName string
	DefaultEntrypointAddr string
}

func DefaultOptions() Options {
	return Options{
		DefaultEntrypointName: "web",
		DefaultEntrypointAddr: ":8080",
	}
}

func New(name string, source Source, options Options) *Provider {
	if name == "" {
		name = "k8s"
	}
	defaults := DefaultOptions()
	if options.DefaultEntrypointName == "" {
		options.DefaultEntrypointName = defaults.DefaultEntrypointName
	}
	if options.DefaultEntrypointAddr == "" {
		options.DefaultEntrypointAddr = defaults.DefaultEntrypointAddr
	}
	return &Provider{
		name:    name,
		source:  source,
		options: options,
	}
}

func (p *Provider) SetLogger(logger *slog.Logger) {
	p.logger = logger
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(ctx context.Context) (*config.Config, error) {
	if p.source == nil {
		return nil, fmt.Errorf("k8s provider source is nil")
	}

	routes, err := p.source.ListRoutes(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("k8s route list failed", "provider", p.name, "error", err)
		}
		return nil, err
	}
	endpoints, err := p.source.ListEndpoints(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("k8s endpoint list failed", "provider", p.name, "error", err)
		}
		return nil, err
	}
	if p.logger != nil {
		p.logger.Info("k8s resources listed", "provider", p.name, "routes", len(routes), "endpoints", len(endpoints))
	}

	cfg := provider.NewEntrypointConfig(p.options.DefaultEntrypointName, p.options.DefaultEntrypointAddr)

	serviceMap := mapping.NewMap[string, *config.Service]()
	invalidEndpointCount := 0
	for _, endpoint := range endpoints {
		serviceName := strings.TrimSpace(endpoint.Service)
		if serviceName == "" || endpoint.URL == "" {
			invalidEndpointCount++
			continue
		}
		service, _ := serviceMap.GetOrCompute(serviceName, func() *config.Service {
			return &config.Service{
				Name:      serviceName,
				Strategy:  "round_robin",
				Endpoints: nil,
			}
		})
		weight := endpoint.Weight
		if weight <= 0 {
			weight = 1
		}
		service.Endpoints = append(service.Endpoints, config.Endpoint{
			URL:    endpoint.URL,
			Weight: weight,
		})
	}

	invalidRouteCount := 0
	middlewareMap := mapping.NewMap[string, config.Middleware]()
	for _, route := range routes {
		entrypoint := route.Entrypoint
		if strings.TrimSpace(entrypoint) == "" {
			entrypoint = p.options.DefaultEntrypointName
		}
		method := strings.TrimSpace(route.Method)
		if strings.TrimSpace(route.Name) == "" || strings.TrimSpace(route.Service) == "" {
			invalidRouteCount++
			continue
		}
		for _, middleware := range route.MiddlewareConfigs {
			if strings.TrimSpace(middleware.Name) != "" {
				middlewareMap.Set(middleware.Name, middleware)
			}
		}
		cfg.Routes = append(cfg.Routes, config.Route{
			Name:        route.Name,
			Entrypoint:  entrypoint,
			Service:     route.Service,
			Host:        route.Host,
			PathPrefix:  route.PathPrefix,
			Method:      method,
			Headers:     route.Headers,
			Middlewares: route.Middlewares,
		})
	}

	provider.AppendSortedServices(cfg, serviceMap)
	for _, middlewareName := range provider.SortedStrings(middlewareMap.Keys()) {
		middleware, _ := middlewareMap.Get(middlewareName)
		cfg.Middlewares = append(cfg.Middlewares, middleware)
	}
	slices.SortStableFunc(cfg.Routes, func(i, j config.Route) int {
		return strings.Compare(i.Name, j.Name)
	})

	if err := config.Validate(cfg); err != nil {
		if p.logger != nil {
			p.logger.Error("k8s config validation failed", "provider", p.name, "error", err)
		}
		return nil, err
	}
	if p.logger != nil {
		p.logger.Info("k8s config built",
			"provider", p.name,
			"routes_seen", len(routes),
			"endpoints_seen", len(endpoints),
			"invalid_routes", invalidRouteCount,
			"invalid_endpoints", invalidEndpointCount,
			"middlewares", len(cfg.Middlewares),
			"services", len(cfg.Services),
			"routes", len(cfg.Routes),
		)
	}
	return cfg, nil
}

func (p *Provider) Watch(ctx context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	if p.source == nil {
		return nil, fmt.Errorf("k8s provider source is nil")
	}
	return p.source.Watch(ctx, onReload, onError)
}

type MemorySource struct {
	mu        sync.RWMutex
	routes    []HTTPRoute
	endpoints []ServiceEndpoint
	listeners *mapping.Map[int, func()]
	nextID    int
}

func NewMemorySource(routes []HTTPRoute, endpoints []ServiceEndpoint) *MemorySource {
	return &MemorySource{
		routes:    routes,
		endpoints: endpoints,
		listeners: mapping.NewMap[int, func()](),
	}
}

func (s *MemorySource) ListRoutes(_ context.Context) ([]HTTPRoute, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]HTTPRoute, len(s.routes))
	copy(out, s.routes)
	return out, nil
}

func (s *MemorySource) ListEndpoints(_ context.Context) ([]ServiceEndpoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ServiceEndpoint, len(s.endpoints))
	copy(out, s.endpoints)
	return out, nil
}

func (s *MemorySource) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	s.listeners.Set(id, onReload)
	return provider.NewOnceCloser(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.listeners.Delete(id)
	}), nil
}

func (s *MemorySource) Update(routes []HTTPRoute, endpoints []ServiceEndpoint) {
	s.mu.Lock()
	s.routes = routes
	s.endpoints = endpoints
	listeners := s.listeners.Values()
	s.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
}

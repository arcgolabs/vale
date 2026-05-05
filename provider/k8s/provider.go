package k8s

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/arcgolabs/vela/config"
)

type HTTPRoute struct {
	Name       string
	Entrypoint string
	Host       string
	PathPrefix string
	Method     string
	Headers    map[string]string
	Service    string
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

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(ctx context.Context) (*config.Config, error) {
	if p.source == nil {
		return nil, fmt.Errorf("k8s provider source is nil")
	}

	routes, err := p.source.ListRoutes(ctx)
	if err != nil {
		return nil, err
	}
	endpoints, err := p.source.ListEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{
			{
				Name:    p.options.DefaultEntrypointName,
				Address: p.options.DefaultEntrypointAddr,
			},
		},
		Services: make([]config.Service, 0),
		Routes:   make([]config.Route, 0),
	}

	serviceMap := make(map[string]*config.Service)
	for _, endpoint := range endpoints {
		serviceName := strings.TrimSpace(endpoint.Service)
		if serviceName == "" || endpoint.URL == "" {
			continue
		}
		service, exists := serviceMap[serviceName]
		if !exists {
			service = &config.Service{
				Name:      serviceName,
				Strategy:  "round_robin",
				Endpoints: make([]config.Endpoint, 0),
			}
			serviceMap[serviceName] = service
		}
		weight := endpoint.Weight
		if weight <= 0 {
			weight = 1
		}
		service.Endpoints = append(service.Endpoints, config.Endpoint{
			URL:    endpoint.URL,
			Weight: weight,
		})
	}

	for _, route := range routes {
		entrypoint := route.Entrypoint
		if strings.TrimSpace(entrypoint) == "" {
			entrypoint = p.options.DefaultEntrypointName
		}
		method := strings.TrimSpace(route.Method)
		if strings.TrimSpace(route.Name) == "" || strings.TrimSpace(route.Service) == "" {
			continue
		}
		cfg.Routes = append(cfg.Routes, config.Route{
			Name:       route.Name,
			Entrypoint: entrypoint,
			Service:    route.Service,
			Host:       route.Host,
			PathPrefix: route.PathPrefix,
			Method:     method,
			Headers:    route.Headers,
		})
	}

	for _, serviceName := range sortedServiceKeys(serviceMap) {
		cfg.Services = append(cfg.Services, *serviceMap[serviceName])
	}
	sort.SliceStable(cfg.Routes, func(i, j int) bool {
		return cfg.Routes[i].Name < cfg.Routes[j].Name
	})

	if err := config.Validate(cfg); err != nil {
		return nil, err
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
	listeners map[int]func()
	nextID    int
}

func NewMemorySource(routes []HTTPRoute, endpoints []ServiceEndpoint) *MemorySource {
	return &MemorySource{
		routes:    routes,
		endpoints: endpoints,
		listeners: make(map[int]func()),
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
	s.listeners[id] = onReload
	return &watchCloser{
		closeFn: func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			delete(s.listeners, id)
		},
	}, nil
}

func (s *MemorySource) Update(routes []HTTPRoute, endpoints []ServiceEndpoint) {
	s.mu.Lock()
	s.routes = routes
	s.endpoints = endpoints
	listeners := make([]func(), 0, len(s.listeners))
	for _, listener := range s.listeners {
		listeners = append(listeners, listener)
	}
	s.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
}

type watchCloser struct {
	once    sync.Once
	closeFn func()
}

func (c *watchCloser) Close() error {
	c.once.Do(func() {
		if c.closeFn != nil {
			c.closeFn()
		}
	})
	return nil
}

func sortedServiceKeys(source map[string]*config.Service) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

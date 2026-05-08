// Package k8s provides a Kubernetes-like config provider abstraction for Vela.
package k8s

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/samber/oops"
)

type HTTPRoute struct {
	Name              string
	Entrypoint        string
	Host              string
	PathPrefix        string
	Method            string
	Headers           *mapping.Map[string, string]
	Middlewares       *collectionlist.List[string]
	MiddlewareConfigs *collectionlist.List[config.Middleware]
	Service           string
}

type ServiceEndpoint struct {
	Service string
	URL     string
	Weight  int
}

type Source interface {
	ListRoutes(context.Context) (*collectionlist.List[HTTPRoute], error)
	ListEndpoints(context.Context) (*collectionlist.List[ServiceEndpoint], error)
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
		return nil, errors.New("k8s provider source is nil")
	}

	routes, endpoints, err := p.listResources(ctx)
	if err != nil {
		return nil, err
	}
	result := p.buildConfig(routes, endpoints)
	if err := config.Validate(result.Config); err != nil {
		if p.logger != nil {
			p.logger.Error("k8s config validation failed", "provider", p.name, "error", err)
		}
		return nil, oops.In("k8s_provider").With("provider", p.name).Wrapf(err, "validate k8s config")
	}
	p.logConfigBuilt(result, routes, endpoints)
	return result.Config, nil
}

func (p *Provider) listResources(ctx context.Context) (
	*collectionlist.List[HTTPRoute],
	*collectionlist.List[ServiceEndpoint],
	error,
) {
	routes, err := p.source.ListRoutes(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("k8s route list failed", "provider", p.name, "error", err)
		}
		return nil, nil, oops.In("k8s_provider").With("provider", p.name).Wrapf(err, "list k8s routes")
	}
	endpoints, err := p.source.ListEndpoints(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("k8s endpoint list failed", "provider", p.name, "error", err)
		}
		return nil, nil, oops.In("k8s_provider").With("provider", p.name).Wrapf(err, "list k8s endpoints")
	}
	if p.logger != nil {
		p.logger.Info("k8s resources listed", "provider", p.name, "routes", routes.Len(), "endpoints", endpoints.Len())
	}
	return routes, endpoints, nil
}

type k8sBuildResult struct {
	Config               *config.Config
	InvalidEndpointCount int
	InvalidRouteCount    int
	ServiceMap           *mapping.Map[string, *config.Service]
	MiddlewareMap        *mapping.Map[string, config.Middleware]
}

func (p *Provider) buildConfig(
	routes *collectionlist.List[HTTPRoute],
	endpoints *collectionlist.List[ServiceEndpoint],
) k8sBuildResult {
	result := k8sBuildResult{
		Config:        provider.NewEntrypointConfig(p.options.DefaultEntrypointName, p.options.DefaultEntrypointAddr),
		ServiceMap:    mapping.NewMap[string, *config.Service](),
		MiddlewareMap: mapping.NewMap[string, config.Middleware](),
	}
	endpoints.Range(func(_ int, endpoint ServiceEndpoint) bool {
		result.addEndpoint(endpoint)
		return true
	})
	routes.Range(func(_ int, route HTTPRoute) bool {
		result.addRoute(route, p.options.DefaultEntrypointName)
		return true
	})
	provider.AppendSortedServices(result.Config, result.ServiceMap)
	provider.SortedStrings(collectionlist.NewList(result.MiddlewareMap.Keys()...)).Range(func(_ int, middlewareName string) bool {
		middleware, _ := result.MiddlewareMap.Get(middlewareName)
		result.Config.Middlewares = append(result.Config.Middlewares, middleware)
		return true
	})
	slices.SortStableFunc(result.Config.Routes, func(i, j config.Route) int {
		return strings.Compare(i.Name, j.Name)
	})
	return result
}

func (r *k8sBuildResult) addEndpoint(endpoint ServiceEndpoint) {
	serviceName := strings.TrimSpace(endpoint.Service)
	if serviceName == "" || endpoint.URL == "" {
		r.InvalidEndpointCount++
		return
	}
	service, _ := r.ServiceMap.GetOrCompute(serviceName, func() *config.Service {
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

func (r *k8sBuildResult) addRoute(route HTTPRoute, defaultEntrypoint string) {
	entrypoint := route.Entrypoint
	if strings.TrimSpace(entrypoint) == "" {
		entrypoint = defaultEntrypoint
	}
	method := strings.TrimSpace(route.Method)
	if strings.TrimSpace(route.Name) == "" || strings.TrimSpace(route.Service) == "" {
		r.InvalidRouteCount++
		return
	}
	route.MiddlewareConfigs.Range(func(_ int, middleware config.Middleware) bool {
		if strings.TrimSpace(middleware.Name) != "" {
			r.MiddlewareMap.Set(middleware.Name, middleware)
		}
		return true
	})
	r.Config.Routes = append(r.Config.Routes, config.Route{
		Name:        route.Name,
		Entrypoint:  entrypoint,
		Service:     route.Service,
		Host:        route.Host,
		PathPrefix:  route.PathPrefix,
		Method:      method,
		Headers:     route.Headers.All(),
		Middlewares: route.Middlewares.Values(),
	})
}

func (p *Provider) logConfigBuilt(
	result k8sBuildResult,
	routes *collectionlist.List[HTTPRoute],
	endpoints *collectionlist.List[ServiceEndpoint],
) {
	if p.logger != nil {
		p.logger.Info("k8s config built",
			"provider", p.name,
			"routes_seen", routes.Len(),
			"endpoints_seen", endpoints.Len(),
			"invalid_routes", result.InvalidRouteCount,
			"invalid_endpoints", result.InvalidEndpointCount,
			"middlewares", len(result.Config.Middlewares),
			"services", len(result.Config.Services),
			"routes", len(result.Config.Routes),
		)
	}
}

func (p *Provider) Watch(ctx context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	if p.source == nil {
		return nil, errors.New("k8s provider source is nil")
	}
	closer, err := p.source.Watch(ctx, onReload, onError)
	if err != nil {
		return nil, oops.In("k8s_provider").With("provider", p.name).Wrapf(err, "watch k8s source")
	}
	return closer, nil
}

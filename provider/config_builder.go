package provider

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
)

type EntrypointOption func(*config.Entrypoint)
type RouteOption func(*config.Route)
type MiddlewareOption func(*config.Middleware)

type ConfigBuilder struct {
	entrypoints *collectionlist.List[config.Entrypoint]
	services    *collectionlist.List[config.Service]
	middlewares *collectionlist.List[config.Middleware]
	routes      *collectionlist.List[config.Route]
	cfg         *config.Config
	errors      *collectionlist.List[error]
}

func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		entrypoints: collectionlist.NewList[config.Entrypoint](),
		services:    collectionlist.NewList[config.Service](),
		middlewares: collectionlist.NewList[config.Middleware](),
		routes:      collectionlist.NewList[config.Route](),
		cfg:         &config.Config{},
		errors:      collectionlist.NewList[error](),
	}
}

func ConfigEndpoint(rawURL string, weight int) config.Endpoint {
	if weight <= 0 {
		weight = 1
	}
	return config.Endpoint{URL: strings.TrimSpace(rawURL), Weight: weight}
}

func (b *ConfigBuilder) Entrypoint(name string, address string, options ...EntrypointOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	entrypoint := config.Entrypoint{Name: strings.TrimSpace(name), Address: strings.TrimSpace(address)}
	if entrypoint.Name == "" {
		b.addError("entrypoint name cannot be empty")
	}
	if entrypoint.Address == "" {
		b.addError("entrypoint %q address cannot be empty", entrypoint.Name)
	}
	collectionlist.NewList(options...).Range(func(_ int, option EntrypointOption) bool {
		if option != nil {
			option(&entrypoint)
		}
		return true
	})
	b.entrypoints.Add(entrypoint)
	return b
}

func EntrypointTLS(certFile string, keyFile string) EntrypointOption {
	return func(entrypoint *config.Entrypoint) {
		if entrypoint == nil {
			return
		}
		entrypoint.TLS = &config.EntrypointTLS{
			Enabled:  true,
			CertFile: strings.TrimSpace(certFile),
			KeyFile:  strings.TrimSpace(keyFile),
		}
	}
}

func EntrypointACME(email string, cacheDir string, domains ...string) EntrypointOption {
	return func(entrypoint *config.Entrypoint) {
		if entrypoint == nil {
			return
		}
		domainList := collectionlist.NewListWithCapacity[string](len(domains))
		for _, domain := range domains {
			if trimmed := strings.TrimSpace(domain); trimmed != "" {
				domainList.Add(trimmed)
			}
		}
		entrypoint.ACME = &config.EntrypointACME{
			Enabled:  true,
			Email:    strings.TrimSpace(email),
			CacheDir: strings.TrimSpace(cacheDir),
			Domains:  domainList.Values(),
		}
	}
}

func (b *ConfigBuilder) Service(name string, endpointURL string) *ConfigBuilder {
	return b.ServiceWithEndpoints(name, ConfigEndpoint(endpointURL, 1))
}

func (b *ConfigBuilder) ServiceWithEndpoints(name string, endpoints ...config.Endpoint) *ConfigBuilder {
	return b.ServiceWithStrategy(name, "round_robin", endpoints...)
}

func (b *ConfigBuilder) ServiceWithStrategy(name string, strategy string, endpoints ...config.Endpoint) *ConfigBuilder {
	if b == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	strategy = strings.TrimSpace(strategy)
	if name == "" {
		b.addError("service name cannot be empty")
	}
	if strategy == "" {
		strategy = "round_robin"
	}
	if strategy != "round_robin" && strategy != "weighted_round_robin" {
		b.addError("service %q has unsupported strategy %q", name, strategy)
	}
	if len(endpoints) == 0 {
		b.addError("service %q must have at least one endpoint", name)
	}
	endpointList := collectionlist.NewListWithCapacity[config.Endpoint](len(endpoints))
	for _, endpoint := range endpoints {
		endpoint.URL = strings.TrimSpace(endpoint.URL)
		if endpoint.URL == "" {
			b.addError("service %q endpoint url cannot be empty", name)
		} else if parsed, err := url.Parse(endpoint.URL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
			b.addError("service %q endpoint %q is invalid", name, endpoint.URL)
		}
		if endpoint.Weight <= 0 {
			endpoint.Weight = 1
		}
		endpointList.Add(endpoint)
	}
	b.services.Add(config.Service{
		Name:      name,
		Strategy:  strategy,
		Endpoints: endpointList.Values(),
	})
	return b
}

func (b *ConfigBuilder) MiddlewareNamed(name string, options ...MiddlewareOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	middleware := config.Middleware{
		Name:            strings.TrimSpace(name),
		RequestHeaders:  map[string]string{},
		ResponseHeaders: map[string]string{},
	}
	if middleware.Name == "" {
		b.addError("middleware name cannot be empty")
	}
	collectionlist.NewList(options...).Range(func(_ int, option MiddlewareOption) bool {
		if option != nil {
			option(&middleware)
		}
		return true
	})
	b.middlewares.Add(middleware)
	return b
}

func (b *ConfigBuilder) Middleware(middleware config.Middleware) *ConfigBuilder {
	if b == nil {
		return nil
	}
	if strings.TrimSpace(middleware.Name) == "" {
		b.addError("middleware name cannot be empty")
	}
	b.middlewares.Add(middleware)
	return b
}

func MiddlewareType(middlewareType string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.Type = strings.TrimSpace(middlewareType)
		}
	}
}

func MiddlewareStripPrefix(pathPrefix string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.StripPrefix = strings.TrimSpace(pathPrefix)
		}
	}
}

func MiddlewareAddPrefix(pathPrefix string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.AddPrefix = strings.TrimSpace(pathPrefix)
		}
	}
}

func MiddlewareRequestHeader(key string, value string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		if middleware.RequestHeaders == nil {
			middleware.RequestHeaders = map[string]string{}
		}
		middleware.RequestHeaders[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func MiddlewareResponseHeader(key string, value string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		if middleware.ResponseHeaders == nil {
			middleware.ResponseHeaders = map[string]string{}
		}
		middleware.ResponseHeaders[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func MiddlewareMaxBodyBytes(maxBodyBytes int64) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.MaxBodyBytes = maxBodyBytes
		}
	}
}

func (b *ConfigBuilder) RouteTo(name string, entrypoint string, service string, options ...RouteOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	route := config.Route{
		Name:       strings.TrimSpace(name),
		Entrypoint: strings.TrimSpace(entrypoint),
		Service:    strings.TrimSpace(service),
		Headers:    map[string]string{},
	}
	if route.Name == "" {
		b.addError("route name cannot be empty")
	}
	if route.Entrypoint == "" {
		b.addError("route %q entrypoint cannot be empty", route.Name)
	}
	if route.Service == "" {
		b.addError("route %q service cannot be empty", route.Name)
	}
	collectionlist.NewList(options...).Range(func(_ int, option RouteOption) bool {
		if option != nil {
			option(&route)
		}
		return true
	})
	b.routes.Add(route)
	return b
}

func (b *ConfigBuilder) Route(route config.Route) *ConfigBuilder {
	if b == nil {
		return nil
	}
	if strings.TrimSpace(route.Name) == "" {
		b.addError("route name cannot be empty")
	}
	b.routes.Add(route)
	return b
}

func RouteHost(host string) RouteOption {
	return func(route *config.Route) {
		if route != nil {
			route.Host = strings.TrimSpace(host)
		}
	}
}

func RoutePathPrefix(pathPrefix string) RouteOption {
	return func(route *config.Route) {
		if route != nil {
			route.PathPrefix = strings.TrimSpace(pathPrefix)
		}
	}
}

func RouteMethod(method string) RouteOption {
	return func(route *config.Route) {
		if route != nil {
			route.Method = strings.TrimSpace(method)
		}
	}
}

func RouteHeader(key string, value string) RouteOption {
	return func(route *config.Route) {
		if route == nil {
			return
		}
		if route.Headers == nil {
			route.Headers = map[string]string{}
		}
		route.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func RouteMiddlewares(names ...string) RouteOption {
	return func(route *config.Route) {
		if route == nil {
			return
		}
		nameList := collectionlist.NewListWithCapacity[string](len(names))
		for _, name := range names {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				nameList.Add(trimmed)
			}
		}
		route.Middlewares = nameList.Values()
	}
}

func (b *ConfigBuilder) Admin(address string) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Admin = &config.Admin{Address: strings.TrimSpace(address)}
	return b
}

func (b *ConfigBuilder) Observability(accessLog bool, metrics bool) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Observability = &config.Observability{AccessLog: accessLog, Metrics: metrics}
	return b
}

func (b *ConfigBuilder) Health(interval string, timeout string) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Health = &config.Health{
		Interval: strings.TrimSpace(interval),
		Timeout:  strings.TrimSpace(timeout),
	}
	return b
}

func (b *ConfigBuilder) Security(security config.Security) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Security = &security
	return b
}

func (b *ConfigBuilder) Build() *config.Config {
	if b == nil {
		return &config.Config{}
	}
	cfg := *b.cfg
	cfg.Entrypoints = b.entrypoints.Values()
	cfg.Services = b.services.Values()
	cfg.Middlewares = b.middlewares.Values()
	cfg.Routes = b.routes.Values()
	return &cfg
}

func (b *ConfigBuilder) BuildValidated() (*config.Config, error) {
	if b == nil {
		return &config.Config{}, errors.New("config builder is nil")
	}
	cfg := b.Build()
	errs := collectionlist.NewList[error]()
	errs.Merge(b.errors)
	if err := config.Validate(cfg); err != nil {
		errs.Add(err)
	}
	return cfg, errors.Join(errs.Values()...)
}

func (b *ConfigBuilder) Errors() *collectionlist.List[error] {
	if b == nil || b.errors == nil {
		return collectionlist.NewList[error]()
	}
	return b.errors.Clone()
}

func (b *ConfigBuilder) addError(format string, args ...any) {
	if b == nil {
		return
	}
	if b.errors == nil {
		b.errors = collectionlist.NewList[error]()
	}
	b.errors.Add(fmt.Errorf(format, args...))
}

func NewEntrypointConfig(name string, address string) *config.Config {
	return &config.Config{
		Entrypoints: []config.Entrypoint{
			{
				Name:    name,
				Address: address,
			},
		},
		Middlewares: make([]config.Middleware, 0),
		Services:    make([]config.Service, 0),
		Routes:      make([]config.Route, 0),
	}
}

func AppendSortedServices(cfg *config.Config, services *mapping.Map[string, *config.Service]) {
	if cfg == nil || services == nil {
		return
	}
	for _, serviceName := range SortedStrings(services.Keys()) {
		service, _ := services.Get(serviceName)
		if service != nil {
			cfg.Services = append(cfg.Services, *service)
		}
	}
}

func AppendSortedRoutes(cfg *config.Config, routes *mapping.Map[string, config.Route]) {
	if cfg == nil || routes == nil {
		return
	}
	for _, routeName := range SortedStrings(routes.Keys()) {
		route, _ := routes.Get(routeName)
		cfg.Routes = append(cfg.Routes, route)
	}
}

package compiler

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/proxy"
	"github.com/arcgolabs/vale/runtime"
)

const DefaultACMECacheDir = ".vale/acme"

func Compile(cfg *config.Config) (*runtime.CompiledSnapshot, error) {
	middlewareMap, err := compileMiddlewares(cfg.Middlewares)
	if err != nil {
		return nil, err
	}

	serviceMap, err := compileServices(cfg.Services)
	if err != nil {
		return nil, err
	}
	entrypointMap, entrypointConfigMap := compileEntrypoints(cfg.Entrypoints)
	routesByEntrypoint := compileRoutes(cfg.Routes, serviceMap, middlewareMap)
	snapshot := &runtime.CompiledSnapshot{
		Entrypoints:        entrypointMap,
		EntrypointConfigs:  entrypointConfigMap,
		RoutesByEntrypoint: routesByEntrypoint,
		EntrypointMatchers: compileEntrypointMatchers(routesByEntrypoint),
		Services:           serviceMap,
		AdminAddress:       pickAdminAddress(cfg),
		AccessLogEnabled:   pickAccessLogEnabled(cfg),
		MetricsEnabled:     pickMetricsEnabled(cfg),
		HealthInterval:     pickHealthInterval(cfg),
		HealthTimeout:      pickHealthTimeout(cfg),
		Security:           pickSecurity(cfg),
		ProxyEngine:        proxy.DefaultEngine.Name(),
		BuiltAt:            time.Now(),
	}
	snapshot.BuildCatalog()
	return snapshot, nil
}

func compileServices(services []config.Service) (*mapping.Map[string, *runtime.ServiceRuntime], error) {
	serviceMap := mapping.NewMapWithCapacity[string, *runtime.ServiceRuntime](len(services))
	for index := range services {
		service := &services[index]
		rtService, err := compileService(service)
		if err != nil {
			return nil, err
		}
		serviceMap.Set(rtService.Name, rtService)
	}
	return serviceMap, nil
}

func compileService(service *config.Service) (*runtime.ServiceRuntime, error) {
	strategy := strings.TrimSpace(service.Strategy)
	if strategy == "" {
		strategy = "round_robin"
	}
	if strategy != "round_robin" && strategy != "weighted_round_robin" {
		return nil, fmt.Errorf("service %q has unsupported strategy %q", service.Name, strategy)
	}
	rtService := &runtime.ServiceRuntime{
		Name:      service.Name,
		Strategy:  strategy,
		Endpoints: collectionlist.NewListWithCapacity[*runtime.EndpointRuntime](len(service.Endpoints)),
	}
	for _, endpoint := range service.Endpoints {
		rtEndpoint, err := compileEndpoint(service.Name, endpoint)
		if err != nil {
			return nil, err
		}
		rtService.Endpoints.Add(rtEndpoint)
	}
	rtService.BuildSlots()
	return rtService, nil
}

func compileEndpoint(serviceName string, endpoint config.Endpoint) (*runtime.EndpointRuntime, error) {
	parsedURL, err := url.Parse(endpoint.URL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("service %q endpoint %q is invalid", serviceName, endpoint.URL)
	}
	weight := endpoint.Weight
	if weight <= 0 {
		weight = 1
	}
	rtEndpoint := &runtime.EndpointRuntime{
		URL:    parsedURL,
		Weight: weight,
		Proxy:  proxy.Build(parsedURL),
	}
	rtEndpoint.Healthy.Store(true)
	return rtEndpoint, nil
}

func compileEntrypoints(entrypoints []config.Entrypoint) (*mapping.Map[string, string], *mapping.Map[string, runtime.EntrypointRuntime]) {
	entrypointMap := mapping.NewMapWithCapacity[string, string](len(entrypoints))
	entrypointConfigMap := mapping.NewMapWithCapacity[string, runtime.EntrypointRuntime](len(entrypoints))
	for _, entrypoint := range entrypoints {
		entrypointMap.Set(entrypoint.Name, entrypoint.Address)
		entrypointConfigMap.Set(entrypoint.Name, compileEntrypoint(entrypoint))
	}
	return entrypointMap, entrypointConfigMap
}

func compileRoutes(
	routes []config.Route,
	serviceMap *mapping.Map[string, *runtime.ServiceRuntime],
	middlewareMap *mapping.Map[string, runtime.MiddlewareRuntime],
) *mapping.MultiMap[string, *runtime.CompiledRoute] {
	routesByEntrypoint := mapping.NewMultiMap[string, *runtime.CompiledRoute]()
	for index := range routes {
		route := &routes[index]
		service, _ := serviceMap.Get(route.Service)
		routesByEntrypoint.Put(route.Entrypoint, compileRoute(route, service, middlewareMap))
	}
	return routesByEntrypoint
}

func compileRoute(
	route *config.Route,
	service *runtime.ServiceRuntime,
	middlewareMap *mapping.Map[string, runtime.MiddlewareRuntime],
) *runtime.CompiledRoute {
	return &runtime.CompiledRoute{
		Name:        route.Name,
		Entrypoint:  route.Entrypoint,
		Host:        strings.ToLower(strings.TrimSpace(route.Host)),
		PathPrefix:  strings.TrimSpace(route.PathPrefix),
		Method:      strings.ToUpper(strings.TrimSpace(route.Method)),
		Headers:     normalizeHeaders(route.Headers),
		Service:     service,
		Predicates:  compileRoutePredicates(*route),
		Middlewares: compileRouteMiddlewares(route.Middlewares, middlewareMap),
	}
}

func compileEntrypointMatchers(
	routesByEntrypoint *mapping.MultiMap[string, *runtime.CompiledRoute],
) *mapping.Map[string, *runtime.EntrypointMatcher] {
	matcherMap := mapping.NewMapWithCapacity[string, *runtime.EntrypointMatcher](routesByEntrypoint.Len())
	routesByEntrypoint.Range(func(entrypoint string, entrypointRoutes []*runtime.CompiledRoute) bool {
		matcherMap.Set(entrypoint, runtime.BuildEntrypointMatcher(collectionlist.NewList(entrypointRoutes...)))
		return true
	})
	return matcherMap
}

func normalizeHeaders(headers map[string]string) *mapping.Map[string, string] {
	headerMap := mapping.NewMapWithCapacity[string, string](len(headers))
	for key, value := range headers {
		headerMap.Set(strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value))
	}
	return headerMap
}

func compileEntrypoint(entrypoint config.Entrypoint) runtime.EntrypointRuntime {
	return runtime.EntrypointRuntime{
		Name:    entrypoint.Name,
		Address: entrypoint.Address,
		TLS:     compileTLS(entrypoint),
	}
}

func compileTLS(entrypoint config.Entrypoint) runtime.TLSRuntime {
	var tlsRuntime runtime.TLSRuntime
	if entrypoint.TLS != nil {
		tlsRuntime.Enabled = entrypoint.TLS.Enabled || entrypoint.TLS.CertFile != "" || entrypoint.TLS.KeyFile != ""
		tlsRuntime.CertFile = strings.TrimSpace(entrypoint.TLS.CertFile)
		tlsRuntime.KeyFile = strings.TrimSpace(entrypoint.TLS.KeyFile)
	}
	if entrypoint.ACME != nil {
		tlsRuntime.Enabled = tlsRuntime.Enabled || entrypoint.ACME.Enabled
		cacheDir := strings.TrimSpace(entrypoint.ACME.CacheDir)
		if entrypoint.ACME.Enabled && cacheDir == "" {
			cacheDir = DefaultACMECacheDir
		}
		tlsRuntime.ACME = runtime.ACMERuntime{
			Enabled:  entrypoint.ACME.Enabled,
			Email:    strings.TrimSpace(entrypoint.ACME.Email),
			CacheDir: cacheDir,
			Domains:  collectionlist.NewList(entrypoint.ACME.Domains...),
		}
	}
	return tlsRuntime
}

func compileRoutePredicates(route config.Route) *bitset.BitSet {
	predicates := bitset.New()
	if strings.TrimSpace(route.Host) != "" {
		predicates.Set(runtime.PredicateHost)
	}
	if strings.TrimSpace(route.PathPrefix) != "" {
		predicates.Set(runtime.PredicatePathPrefix)
	}
	if strings.TrimSpace(route.Method) != "" {
		predicates.Set(runtime.PredicateMethod)
	}
	if len(route.Headers) > 0 {
		predicates.Set(runtime.PredicateHeaders)
	}
	return predicates
}

func pickAdminAddress(cfg *config.Config) string {
	if cfg.Admin != nil && cfg.Admin.Address != "" {
		return cfg.Admin.Address
	}
	return ":19090"
}

func pickAccessLogEnabled(cfg *config.Config) bool {
	if cfg.Observability == nil {
		return true
	}
	return cfg.Observability.AccessLog
}

func pickMetricsEnabled(cfg *config.Config) bool {
	if cfg.Observability == nil {
		return true
	}
	return cfg.Observability.Metrics
}

func pickHealthInterval(cfg *config.Config) string {
	if cfg.Health == nil || cfg.Health.Interval == "" {
		return "5s"
	}
	return cfg.Health.Interval
}

func pickHealthTimeout(cfg *config.Config) string {
	if cfg.Health == nil || cfg.Health.Timeout == "" {
		return "2s"
	}
	return cfg.Health.Timeout
}

func pickSecurity(cfg *config.Config) runtime.SecurityRuntime {
	security := runtime.SecurityRuntime{
		ReadHeaderTimeout: "5s",
		ReadTimeout:       "30s",
		WriteTimeout:      "30s",
		IdleTimeout:       "120s",
		MaxHeaderBytes:    1 << 20,
		MaxBodyBytes:      32 << 20,
	}
	if cfg.Security == nil {
		return security
	}
	if cfg.Security.ReadHeaderTimeout != "" {
		security.ReadHeaderTimeout = cfg.Security.ReadHeaderTimeout
	}
	if cfg.Security.ReadTimeout != "" {
		security.ReadTimeout = cfg.Security.ReadTimeout
	}
	if cfg.Security.WriteTimeout != "" {
		security.WriteTimeout = cfg.Security.WriteTimeout
	}
	if cfg.Security.IdleTimeout != "" {
		security.IdleTimeout = cfg.Security.IdleTimeout
	}
	if cfg.Security.MaxHeaderBytes > 0 {
		security.MaxHeaderBytes = cfg.Security.MaxHeaderBytes
	}
	if cfg.Security.MaxBodyBytes > 0 {
		security.MaxBodyBytes = cfg.Security.MaxBodyBytes
	}
	return security
}

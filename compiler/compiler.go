package compiler

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/proxy"
	"github.com/arcgolabs/vale/runtime"
)

const DefaultACMECacheDir = ".vale/acme"

type Options struct {
	MiddlewareTypes *collectionlist.List[string]
}

func Compile(cfg *config.Config) (*runtime.CompiledSnapshot, error) {
	return CompileWithOptions(cfg, Options{})
}

func CompileWithOptions(cfg *config.Config, options Options) (*runtime.CompiledSnapshot, error) {
	middlewareMap, err := compileMiddlewares(cfg.Middlewares, options.MiddlewareTypes)
	if err != nil {
		return nil, err
	}

	serviceMap, err := compileServices(cfg.Services)
	if err != nil {
		return nil, err
	}
	tcpServiceMap, err := compileTCPServices(cfg.TCPServices)
	if err != nil {
		return nil, err
	}
	entrypointMap, entrypointConfigMap := compileEntrypoints(cfg.Entrypoints)
	routesByEntrypoint, err := compileRoutes(cfg.Routes, serviceMap, middlewareMap)
	if err != nil {
		return nil, err
	}
	tcpRoutes := compileTCPRoutes(cfg.TCPRoutes, tcpServiceMap)
	snapshot := &runtime.CompiledSnapshot{
		Entrypoints:        entrypointMap,
		EntrypointConfigs:  entrypointConfigMap,
		RoutesByEntrypoint: routesByEntrypoint,
		EntrypointMatchers: compileEntrypointMatchers(routesByEntrypoint),
		TCPRoutes:          tcpRoutes,
		Services:           serviceMap,
		TCPServices:        tcpServiceMap,
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
		Kind:   runtime.EndpointKindHTTP,
		URL:    parsedURL,
		Weight: weight,
		Proxy:  proxy.Build(parsedURL),
	}
	rtEndpoint.Healthy.Store(true)
	return rtEndpoint, nil
}

func compileTCPServices(services []config.TCPService) (*mapping.Map[string, *runtime.TCPServiceRuntime], error) {
	serviceMap := mapping.NewMapWithCapacity[string, *runtime.TCPServiceRuntime](len(services))
	for index := range services {
		service := &services[index]
		rtService, err := compileTCPService(service)
		if err != nil {
			return nil, err
		}
		serviceMap.Set(rtService.Name, rtService)
	}
	return serviceMap, nil
}

func compileTCPService(service *config.TCPService) (*runtime.TCPServiceRuntime, error) {
	strategy := strings.TrimSpace(service.Strategy)
	if strategy == "" {
		strategy = "round_robin"
	}
	if strategy != "round_robin" && strategy != "weighted_round_robin" {
		return nil, fmt.Errorf("tcp_service %q has unsupported strategy %q", service.Name, strategy)
	}
	rtService := &runtime.TCPServiceRuntime{
		Name:      service.Name,
		Strategy:  strategy,
		Endpoints: collectionlist.NewListWithCapacity[*runtime.TCPEndpointRuntime](len(service.Endpoints)),
	}
	for _, endpoint := range service.Endpoints {
		rtEndpoint, err := compileTCPEndpoint(service.Name, endpoint)
		if err != nil {
			return nil, err
		}
		rtService.Endpoints.Add(rtEndpoint)
	}
	rtService.BuildSlots()
	return rtService, nil
}

func compileTCPEndpoint(serviceName string, endpoint config.TCPEndpoint) (*runtime.TCPEndpointRuntime, error) {
	address := strings.TrimSpace(endpoint.Address)
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, fmt.Errorf("tcp_service %q endpoint address %q is invalid", serviceName, endpoint.Address)
	}
	weight := endpoint.Weight
	if weight <= 0 {
		weight = 1
	}
	rtEndpoint := &runtime.TCPEndpointRuntime{Address: address, Weight: weight}
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

func compileTCPRoutes(
	routes []config.TCPRoute,
	serviceMap *mapping.Map[string, *runtime.TCPServiceRuntime],
) *mapping.Map[string, *runtime.CompiledTCPRoute] {
	routesByEntrypoint := mapping.NewMapWithCapacity[string, *runtime.CompiledTCPRoute](len(routes))
	for index := range routes {
		route := &routes[index]
		service, _ := serviceMap.Get(route.Service)
		routesByEntrypoint.Set(route.Entrypoint, compileTCPRoute(route, service))
	}
	return routesByEntrypoint
}

func compileTCPRoute(route *config.TCPRoute, service *runtime.TCPServiceRuntime) *runtime.CompiledTCPRoute {
	return &runtime.CompiledTCPRoute{
		Name:       route.Name,
		Entrypoint: route.Entrypoint,
		Service:    service,
	}
}

func compileRoutes(
	routes []config.Route,
	serviceMap *mapping.Map[string, *runtime.ServiceRuntime],
	middlewareMap *mapping.Map[string, runtime.MiddlewareRuntime],
) (*mapping.MultiMap[string, *runtime.CompiledRoute], error) {
	routesByEntrypoint := mapping.NewMultiMap[string, *runtime.CompiledRoute]()
	for index := range routes {
		route := &routes[index]
		service, _ := serviceMap.Get(route.Service)
		compiled, err := compileRoute(route, service, middlewareMap)
		if err != nil {
			return nil, err
		}
		routesByEntrypoint.Put(route.Entrypoint, compiled)
	}
	return routesByEntrypoint, nil
}

func compileRoute(
	route *config.Route,
	service *runtime.ServiceRuntime,
	middlewareMap *mapping.Map[string, runtime.MiddlewareRuntime],
) (*runtime.CompiledRoute, error) {
	writeTimeout, hasWriteTimeout, err := compileRouteWriteTimeout(route.Name, route.WriteTimeout)
	if err != nil {
		return nil, err
	}
	var compiledWriteTimeout *time.Duration
	if hasWriteTimeout {
		compiledWriteTimeout = new(writeTimeout)
	}
	return &runtime.CompiledRoute{
		Name:         route.Name,
		Entrypoint:   route.Entrypoint,
		WriteTimeout: compiledWriteTimeout,
		Host:         strings.ToLower(strings.TrimSpace(route.Host)),
		PathPrefix:   strings.TrimSpace(route.PathPrefix),
		Method:       strings.ToUpper(strings.TrimSpace(route.Method)),
		Headers:      normalizeHeaders(route.Headers),
		Service:      service,
		Predicates:   compileRoutePredicates(*route),
		Middlewares:  compileRouteMiddlewares(route.Middlewares, middlewareMap),
	}, nil
}

func compileRouteWriteTimeout(routeName, raw string) (time.Duration, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, false, fmt.Errorf("route %q write_timeout %q is invalid: %w", routeName, raw, err)
	}
	if timeout < 0 {
		return 0, false, fmt.Errorf("route %q write_timeout must be non-negative", routeName)
	}
	return timeout, true, nil
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
		Name:     entrypoint.Name,
		Address:  entrypoint.Address,
		Protocol: config.NormalizeEntrypointProtocol(entrypoint.Protocol),
		TLS:      compileTLS(entrypoint),
	}
}

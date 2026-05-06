package compiler

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/bitset"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/proxy"
	"github.com/arcgolabs/vela/runtime"
)

func Compile(cfg *config.Config) (*runtime.CompiledSnapshot, error) {
	middlewareMap := mapping.NewMapWithCapacity[string, runtime.MiddlewareRuntime](len(cfg.Middlewares))
	for _, middleware := range cfg.Middlewares {
		middlewareMap.Set(middleware.Name, runtime.MiddlewareRuntime{
			Name:            middleware.Name,
			StripPrefix:     strings.TrimSpace(middleware.StripPrefix),
			AddPrefix:       strings.TrimSpace(middleware.AddPrefix),
			RequestHeaders:  normalizeHeaders(middleware.RequestHeaders),
			ResponseHeaders: normalizeHeaders(middleware.ResponseHeaders),
			MaxBodyBytes:    middleware.MaxBodyBytes,
		})
	}

	serviceMap := mapping.NewMapWithCapacity[string, *runtime.ServiceRuntime](len(cfg.Services))
	for _, service := range cfg.Services {
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
			Endpoints: make([]*runtime.EndpointRuntime, 0, len(service.Endpoints)),
		}
		for _, endpoint := range service.Endpoints {
			parsedURL, err := url.Parse(endpoint.URL)
			if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
				return nil, fmt.Errorf("service %q endpoint %q is invalid", service.Name, endpoint.URL)
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
			rtService.Endpoints = append(rtService.Endpoints, rtEndpoint)
		}

		rtService.BuildSlots()
		serviceMap.Set(rtService.Name, rtService)
	}
	services := serviceMap.All()

	entrypointMap := mapping.NewMapWithCapacity[string, string](len(cfg.Entrypoints))
	entrypointConfigMap := mapping.NewMapWithCapacity[string, runtime.EntrypointRuntime](len(cfg.Entrypoints))
	for _, entrypoint := range cfg.Entrypoints {
		entrypointMap.Set(entrypoint.Name, entrypoint.Address)
		entrypointConfigMap.Set(entrypoint.Name, compileEntrypoint(entrypoint))
	}
	entrypoints := entrypointMap.All()
	entrypointConfigs := entrypointConfigMap.All()

	routesByEntrypoint := mapping.NewMultiMap[string, *runtime.CompiledRoute]()
	for _, route := range cfg.Routes {
		service := services[route.Service]
		headerMap := normalizeHeaders(route.Headers)
		routesByEntrypoint.Put(route.Entrypoint, &runtime.CompiledRoute{
			Name:        route.Name,
			Entrypoint:  route.Entrypoint,
			Host:        strings.ToLower(strings.TrimSpace(route.Host)),
			PathPrefix:  strings.TrimSpace(route.PathPrefix),
			Method:      strings.ToUpper(strings.TrimSpace(route.Method)),
			Headers:     headerMap,
			Service:     service,
			Predicates:  compileRoutePredicates(route),
			Middlewares: compileRouteMiddlewares(route.Middlewares, middlewareMap),
		})
	}
	routes := routesByEntrypoint.All()
	matcherMap := mapping.NewMapWithCapacity[string, *runtime.EntrypointMatcher](len(routes))
	for entrypoint, entrypointRoutes := range routes {
		matcherMap.Set(entrypoint, runtime.BuildEntrypointMatcher(entrypointRoutes))
	}
	matchers := matcherMap.All()

	return &runtime.CompiledSnapshot{
		Entrypoints:        entrypoints,
		EntrypointConfigs:  entrypointConfigs,
		RoutesByEntrypoint: routes,
		EntrypointMatchers: matchers,
		Services:           services,
		AdminAddress:       pickAdminAddress(cfg),
		AccessLogEnabled:   pickAccessLogEnabled(cfg),
		MetricsEnabled:     pickMetricsEnabled(cfg),
		HealthInterval:     pickHealthInterval(cfg),
		HealthTimeout:      pickHealthTimeout(cfg),
		Security:           pickSecurity(cfg),
		ProxyEngine:        proxy.DefaultEngine.Name(),
		BuiltAt:            time.Now(),
	}, nil
}

func normalizeHeaders(headers map[string]string) map[string]string {
	headerMap := mapping.NewMapWithCapacity[string, string](len(headers))
	for key, value := range headers {
		headerMap.Set(strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value))
	}
	return headerMap.All()
}

func compileRouteMiddlewares(names []string, middlewares *mapping.Map[string, runtime.MiddlewareRuntime]) []runtime.MiddlewareRuntime {
	if len(names) == 0 || middlewares == nil {
		return nil
	}
	compiled := make([]runtime.MiddlewareRuntime, 0, len(names))
	for _, name := range names {
		middleware, ok := middlewares.Get(name)
		if ok {
			compiled = append(compiled, middleware)
		}
	}
	return compiled
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
		tlsRuntime.ACME = runtime.ACMERuntime{
			Enabled:  entrypoint.ACME.Enabled,
			Email:    strings.TrimSpace(entrypoint.ACME.Email),
			CacheDir: strings.TrimSpace(entrypoint.ACME.CacheDir),
			Domains:  entrypoint.ACME.Domains,
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

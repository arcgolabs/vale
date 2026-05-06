package compiler

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/proxy"
	"github.com/arcgolabs/vela/runtime"
	"github.com/samber/lo"
)

func Compile(cfg *config.Config) (*runtime.CompiledSnapshot, error) {
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

	entrypoints := lo.Associate(cfg.Entrypoints, func(entrypoint config.Entrypoint) (string, string) {
		return entrypoint.Name, entrypoint.Address
	})

	routesByEntrypoint := mapping.NewMultiMap[string, *runtime.CompiledRoute]()
	for _, route := range cfg.Routes {
		service := services[route.Service]
		headers := lo.MapEntries(route.Headers, func(key string, value string) (string, string) {
			return strings.ToLower(key), value
		})
		routesByEntrypoint.Put(route.Entrypoint, &runtime.CompiledRoute{
			Name:       route.Name,
			Entrypoint: route.Entrypoint,
			Host:       strings.ToLower(strings.TrimSpace(route.Host)),
			PathPrefix: strings.TrimSpace(route.PathPrefix),
			Method:     strings.ToUpper(strings.TrimSpace(route.Method)),
			Headers:    headers,
			Service:    service,
		})
	}
	routes := routesByEntrypoint.All()
	matchers := lo.MapEntries(routes, func(entrypoint string, entrypointRoutes []*runtime.CompiledRoute) (string, *runtime.EntrypointMatcher) {
		return entrypoint, runtime.BuildEntrypointMatcher(entrypointRoutes)
	})

	return &runtime.CompiledSnapshot{
		Entrypoints:        entrypoints,
		RoutesByEntrypoint: routes,
		EntrypointMatchers: matchers,
		Services:           services,
		AdminAddress:       pickAdminAddress(cfg),
		AccessLogEnabled:   pickAccessLogEnabled(cfg),
		MetricsEnabled:     pickMetricsEnabled(cfg),
		HealthInterval:     pickHealthInterval(cfg),
		HealthTimeout:      pickHealthTimeout(cfg),
		ProxyEngine:        proxy.DefaultEngine.Name(),
		BuiltAt:            time.Now(),
	}, nil
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

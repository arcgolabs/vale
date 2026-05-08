package docker

import (
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/vale/config"
	valeprovider "github.com/arcgolabs/vale/provider"
)

func labelsEnabled(labels *mapping.Map[string, string], traefikLabels valeprovider.TraefikLabels) bool {
	return traefikLabels.Enabled.OrElse(parseBool(labelValue(labels, "vale.enable"), false) || traefikLabels.HasHTTPConfig())
}

func applyTraefikContainerConfig(
	services *mapping.Map[string, *config.Service],
	routes *mapping.Map[string, config.Route],
	middlewares *mapping.Map[string, config.Middleware],
	container Container,
	options Options,
	traefikLabels valeprovider.TraefikLabels,
) {
	addedServices := collectionset.NewSet[string]()
	traefikLabels.Services.Range(func(serviceName string, service valeprovider.TraefikService) bool {
		addTraefikServiceEndpoint(services, addedServices, container, serviceName, service)
		return true
	})
	traefikLabels.Middlewares.Range(func(name string, middleware config.Middleware) bool {
		middlewares.Set(name, middleware)
		return true
	})
	if traefikLabels.Routers.IsEmpty() {
		addDefaultTraefikRoute(routes, container, options, traefikLabels)
		return
	}
	traefikLabels.Routers.Range(func(routerName string, router valeprovider.TraefikRouter) bool {
		serviceName := traefikRouterServiceName(container, router, traefikLabels)
		service, _ := traefikLabels.Services.Get(serviceName)
		addTraefikServiceEndpoint(services, addedServices, container, serviceName, service)
		addTraefikRouterRoutes(routes, routerName, router, serviceName, options)
		return true
	})
}

func addTraefikServiceEndpoint(
	services *mapping.Map[string, *config.Service],
	added *collectionset.Set[string],
	container Container,
	serviceName string,
	traefikService valeprovider.TraefikService,
) {
	if added.Contains(serviceName) {
		return
	}
	added.Add(serviceName)
	service, _ := services.GetOrCompute(serviceName, func() *config.Service {
		return &config.Service{
			Name:     serviceName,
			Strategy: "round_robin",
		}
	})
	port := traefikService.Port
	if port <= 0 {
		port = container.Port
	}
	scheme := valueOr(traefikService.Scheme, "http")
	service.Endpoints = append(service.Endpoints, config.Endpoint{
		URL:    fmt.Sprintf("%s://%s:%d", scheme, container.Address, port),
		Weight: 1,
	})
}

func addDefaultTraefikRoute(
	routes *mapping.Map[string, config.Route],
	container Container,
	options Options,
	traefikLabels valeprovider.TraefikLabels,
) {
	serviceName := defaultTraefikServiceName(container, traefikLabels)
	routeName := sanitizeName(container.Name, "container") + "-route"
	if _, exists := routes.Get(routeName); exists {
		return
	}
	routes.Set(routeName, config.Route{
		Name:       routeName,
		Entrypoint: options.DefaultEntrypointName,
		Service:    serviceName,
		PathPrefix: "/",
		Headers:    map[string]string{},
	})
}

func addTraefikRouterRoutes(
	routes *mapping.Map[string, config.Route],
	routerName string,
	router valeprovider.TraefikRouter,
	serviceName string,
	options Options,
) {
	entrypoints := traefikRouterEntrypoints(router, options)
	entrypoints.Range(func(_ int, entrypoint string) bool {
		routeName := routerName
		if entrypoints.Len() > 1 {
			routeName = routerName + "-" + entrypoint
		}
		if _, exists := routes.Get(routeName); exists {
			return true
		}
		routes.Set(routeName, config.Route{
			Name:        routeName,
			Entrypoint:  entrypoint,
			Service:     serviceName,
			Host:        router.Host,
			PathPrefix:  router.PathPrefix,
			Method:      router.Method,
			Headers:     traefikRouteHeaders(router),
			Middlewares: router.Middlewares.Values(),
		})
		return true
	})
}

func traefikRouterServiceName(container Container, router valeprovider.TraefikRouter, labels valeprovider.TraefikLabels) string {
	if router.Service != "" {
		return router.Service
	}
	return defaultTraefikServiceName(container, labels)
}

func defaultTraefikServiceName(container Container, labels valeprovider.TraefikLabels) string {
	if labels.Services.Len() == 1 {
		serviceName, _ := valeprovider.SortedStrings(collectionlist.NewList(labels.Services.Keys()...)).GetFirst()
		return serviceName
	}
	return sanitizeName(container.Name, "service")
}

func traefikRouterEntrypoints(router valeprovider.TraefikRouter, options Options) *collectionlist.List[string] {
	if router.Entrypoints != nil && !router.Entrypoints.IsEmpty() {
		known := collectionlist.NewList[string]()
		router.Entrypoints.Range(func(_ int, entrypoint string) bool {
			if _, ok := options.EntrypointAddresses.Get(entrypoint); ok {
				known.Add(entrypoint)
			}
			return true
		})
		if !known.IsEmpty() {
			return known
		}
	}
	return collectionlist.NewList[string](options.DefaultEntrypointName)
}

func traefikRouteHeaders(router valeprovider.TraefikRouter) map[string]string {
	headers := map[string]string{}
	if router.Headers == nil {
		return headers
	}
	router.Headers.Range(func(key string, value string) bool {
		headers[key] = value
		return true
	})
	return headers
}

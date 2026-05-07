package docker

import (
	"fmt"
	"strings"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
)

type configBuildResult struct {
	Config               *config.Config
	DisabledCount        int
	InvalidEndpointCount int
	ServiceMap           *mapping.Map[string, *config.Service]
	RouteMap             *mapping.Map[string, config.Route]
	MiddlewareMap        *mapping.Map[string, config.Middleware]
}

func buildConfig(options Options, containers []Container) configBuildResult {
	result := configBuildResult{
		Config:        newDockerConfig(options),
		ServiceMap:    mapping.NewMap[string, *config.Service](),
		RouteMap:      mapping.NewMap[string, config.Route](),
		MiddlewareMap: mapping.NewMap[string, config.Middleware](),
	}
	for _, container := range containers {
		result.addContainer(container, options)
	}
	provider.AppendSortedServices(result.Config, result.ServiceMap)
	for _, middlewareName := range provider.SortedStrings(result.MiddlewareMap.Keys()) {
		middleware, _ := result.MiddlewareMap.Get(middlewareName)
		result.Config.Middlewares = append(result.Config.Middlewares, middleware)
	}
	provider.AppendSortedRoutes(result.Config, result.RouteMap)
	return result
}

func (r *configBuildResult) addContainer(container Container, options Options) {
	labels := container.Labels
	traefikLabels := provider.ParseTraefikLabels(labels)
	if !labelsEnabled(labels, traefikLabels) {
		r.DisabledCount++
		return
	}
	if container.Address == "" || container.Port <= 0 {
		r.InvalidEndpointCount++
		return
	}
	applyEntrypointTLSLabels(r.Config, labels)
	if traefikLabels.HasHTTPConfig() {
		applyTraefikContainerConfig(r.ServiceMap, r.RouteMap, r.MiddlewareMap, container, options, traefikLabels)
		return
	}
	r.addVelaContainer(container, options)
}

func (r *configBuildResult) addVelaContainer(container Container, options Options) {
	labels := container.Labels
	serviceName := valueOr(labels["vela.service"], sanitizeName(container.Name, "service"))
	routeName := valueOr(labels["vela.route"], serviceName+"-route")
	routeMiddlewares := provider.SplitCSV(labels["vela.middlewares"])
	if middleware, ok := middlewareFromLabels(routeName+"-middleware", labels); ok {
		r.MiddlewareMap.Set(middleware.Name, middleware)
		routeMiddlewares = append(routeMiddlewares, middleware.Name)
	}
	r.addVelaServiceEndpoint(container, serviceName)
	if _, exists := r.RouteMap.Get(routeName); exists {
		return
	}
	r.RouteMap.Set(routeName, config.Route{
		Name:        routeName,
		Entrypoint:  valueOr(labels["vela.entrypoint"], options.DefaultEntrypointName),
		Service:     serviceName,
		Host:        strings.TrimSpace(labels["vela.rule.host"]),
		PathPrefix:  strings.TrimSpace(labels["vela.rule.pathprefix"]),
		Method:      strings.TrimSpace(labels["vela.rule.method"]),
		Headers:     map[string]string{},
		Middlewares: routeMiddlewares,
	})
}

func (r *configBuildResult) addVelaServiceEndpoint(container Container, serviceName string) {
	labels := container.Labels
	service, _ := r.ServiceMap.GetOrCompute(serviceName, func() *config.Service {
		return &config.Service{
			Name:      serviceName,
			Strategy:  "round_robin",
			Endpoints: nil,
		}
	})
	service.Endpoints = append(service.Endpoints, config.Endpoint{
		URL:    fmt.Sprintf("%s://%s:%d", valueOr(labels["vela.scheme"], "http"), container.Address, container.Port),
		Weight: parseInt(labels["vela.weight"], 1),
	})
}

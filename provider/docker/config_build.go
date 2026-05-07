package docker

import (
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
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

func buildConfig(options Options, containers *collectionlist.List[Container]) configBuildResult {
	result := configBuildResult{
		Config:        newDockerConfig(options),
		ServiceMap:    mapping.NewMap[string, *config.Service](),
		RouteMap:      mapping.NewMap[string, config.Route](),
		MiddlewareMap: mapping.NewMap[string, config.Middleware](),
	}
	containers.Range(func(_ int, container Container) bool {
		result.addContainer(container, options)
		return true
	})
	provider.AppendSortedServices(result.Config, result.ServiceMap)
	provider.SortedStrings(collectionlist.NewList(result.MiddlewareMap.Keys()...)).Range(func(_ int, middlewareName string) bool {
		middleware, _ := result.MiddlewareMap.Get(middlewareName)
		result.Config.Middlewares = append(result.Config.Middlewares, middleware)
		return true
	})
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
	serviceName := valueOr(labelValue(labels, "vela.service"), sanitizeName(container.Name, "service"))
	routeName := valueOr(labelValue(labels, "vela.route"), serviceName+"-route")
	routeMiddlewares := provider.SplitCSV(labelValue(labels, "vela.middlewares"))
	if middleware, ok := middlewareFromLabels(routeName+"-middleware", labels); ok {
		r.MiddlewareMap.Set(middleware.Name, middleware)
		routeMiddlewares.Add(middleware.Name)
	}
	r.addVelaServiceEndpoint(container, serviceName)
	if _, exists := r.RouteMap.Get(routeName); exists {
		return
	}
	r.RouteMap.Set(routeName, config.Route{
		Name:        routeName,
		Entrypoint:  valueOr(labelValue(labels, "vela.entrypoint"), options.DefaultEntrypointName),
		Service:     serviceName,
		Host:        strings.TrimSpace(labelValue(labels, "vela.rule.host")),
		PathPrefix:  strings.TrimSpace(labelValue(labels, "vela.rule.pathprefix")),
		Method:      strings.TrimSpace(labelValue(labels, "vela.rule.method")),
		Headers:     map[string]string{},
		Middlewares: routeMiddlewares.Values(),
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
		URL:    fmt.Sprintf("%s://%s:%d", valueOr(labelValue(labels, "vela.scheme"), "http"), container.Address, container.Port),
		Weight: parseInt(labelValue(labels, "vela.weight"), 1),
	})
}

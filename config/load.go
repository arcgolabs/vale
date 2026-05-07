package config

import (
	"errors"
	"fmt"
	"strings"

	collectiongraph "github.com/arcgolabs/collectionx/graph"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
)

const (
	configNodeEntrypoint = "entrypoint:"
	configNodeMiddleware = "middleware:"
	configNodeRoute      = "route:"
	configNodeService    = "service:"
)

func Validate(cfg *Config) error {
	if err := validateConfigShape(cfg); err != nil {
		return err
	}
	entrypointSet, err := validateEntrypoints(cfg.Entrypoints)
	if err != nil {
		return err
	}
	serviceSet, err := validateServices(cfg.Services)
	if err != nil {
		return err
	}
	middlewareSet, err := validateMiddlewares(cfg.Middlewares)
	if err != nil {
		return err
	}
	refGraph := newReferenceGraph(entrypointSet, serviceSet, middlewareSet)
	if err := validateMiddlewareChains(cfg.Middlewares, refGraph); err != nil {
		return err
	}
	return validateRoutes(cfg.Routes, refGraph)
}

func validateConfigShape(cfg *Config) error {
	if cfg == nil {
		return errors.New("config cannot be nil")
	}
	if len(cfg.Entrypoints) == 0 {
		return errors.New("at least one entrypoint is required")
	}
	if len(cfg.Services) == 0 {
		return errors.New("at least one service is required")
	}
	if len(cfg.Routes) == 0 {
		return errors.New("at least one route is required")
	}
	return nil
}

func validateEntrypoints(entrypoints []Entrypoint) (*collectionset.Set[string], error) {
	entrypointSet := collectionset.NewSetWithCapacity[string](len(entrypoints))
	for index := range entrypoints {
		entrypoint := &entrypoints[index]
		if err := validateEntrypoint(entrypoint, entrypointSet); err != nil {
			return nil, err
		}
		entrypointSet.Add(entrypoint.Name)
	}
	return entrypointSet, nil
}

func validateEntrypoint(entrypoint *Entrypoint, seen *collectionset.Set[string]) error {
	if entrypoint.Name == "" || entrypoint.Address == "" {
		return errors.New("entrypoint name/address cannot be empty")
	}
	if seen.Contains(entrypoint.Name) {
		return fmt.Errorf("duplicated entrypoint %q", entrypoint.Name)
	}
	if err := validateEntrypointTLS(entrypoint); err != nil {
		return err
	}
	return validateEntrypointACME(entrypoint)
}

func validateEntrypointTLS(entrypoint *Entrypoint) error {
	if entrypoint.TLS != nil && entrypoint.TLS.Enabled && (entrypoint.TLS.CertFile == "") != (entrypoint.TLS.KeyFile == "") {
		return fmt.Errorf("entrypoint %q tls requires both cert_file and key_file", entrypoint.Name)
	}
	return nil
}

func validateEntrypointACME(entrypoint *Entrypoint) error {
	if entrypoint.ACME != nil && entrypoint.ACME.Enabled && len(entrypoint.ACME.Domains) == 0 {
		return fmt.Errorf("entrypoint %q acme requires at least one domain", entrypoint.Name)
	}
	if entrypoint.ACME != nil && entrypoint.ACME.Enabled && strings.TrimSpace(entrypoint.ACME.Email) == "" {
		return fmt.Errorf("entrypoint %q acme requires email", entrypoint.Name)
	}
	return nil
}

func validateServices(services []Service) (*collectionset.Set[string], error) {
	serviceSet := collectionset.NewSetWithCapacity[string](len(services))
	for index := range services {
		service := &services[index]
		if err := validateService(service, serviceSet); err != nil {
			return nil, err
		}
		serviceSet.Add(service.Name)
	}
	return serviceSet, nil
}

func validateService(service *Service, seen *collectionset.Set[string]) error {
	if service.Name == "" {
		return errors.New("service name cannot be empty")
	}
	if seen.Contains(service.Name) {
		return fmt.Errorf("duplicated service %q", service.Name)
	}
	if len(service.Endpoints) == 0 {
		return fmt.Errorf("service %q must have at least one endpoint", service.Name)
	}
	for _, endpoint := range service.Endpoints {
		if endpoint.URL == "" {
			return fmt.Errorf("service %q contains empty endpoint url", service.Name)
		}
	}
	return nil
}

func validateMiddlewares(middlewares []Middleware) (*collectionset.Set[string], error) {
	middlewareSet := collectionset.NewSetWithCapacity[string](len(middlewares))
	for index := range middlewares {
		middleware := &middlewares[index]
		if err := validateMiddleware(middleware, middlewareSet); err != nil {
			return nil, err
		}
		middlewareSet.Add(middleware.Name)
	}
	return middlewareSet, nil
}

func newReferenceGraph(
	entrypointSet *collectionset.Set[string],
	serviceSet *collectionset.Set[string],
	middlewareSet *collectionset.Set[string],
) *collectiongraph.Graph[string, string] {
	refGraph := collectiongraph.NewDirectedGraph[string, string]()
	entrypointSet.Range(func(name string) bool {
		refGraph.AddNode(configNodeEntrypoint+name, "entrypoint")
		return true
	})
	serviceSet.Range(func(name string) bool {
		refGraph.AddNode(configNodeService+name, "service")
		return true
	})
	middlewareSet.Range(func(name string) bool {
		refGraph.AddNode(configNodeMiddleware+name, "middleware")
		return true
	})
	return refGraph
}

func validateMiddlewareChains(middlewares []Middleware, refGraph *collectiongraph.Graph[string, string]) error {
	for index := range middlewares {
		middleware := &middlewares[index]
		if err := validateMiddlewareChain(middleware, refGraph); err != nil {
			return err
		}
	}
	return nil
}

func validateMiddlewareChain(middleware *Middleware, refGraph *collectiongraph.Graph[string, string]) error {
	middlewareNode := configNodeMiddleware + middleware.Name
	for _, child := range middleware.Chain {
		child = strings.TrimSpace(child)
		if child == "" {
			continue
		}
		if err := addMiddlewareChainEdge(refGraph, middlewareNode, middleware.Name, child); err != nil {
			return err
		}
	}
	return nil
}

func addMiddlewareChainEdge(
	refGraph *collectiongraph.Graph[string, string],
	middlewareNode, middlewareName, child string,
) error {
	childNode := configNodeMiddleware + child
	if !refGraph.HasNode(childNode) {
		return fmt.Errorf("middleware %q references unknown chain middleware %q", middlewareName, child)
	}
	return addReferenceEdge(refGraph, middlewareNode, childNode)
}

func validateRoutes(routes []Route, refGraph *collectiongraph.Graph[string, string]) error {
	routeSet := collectionset.NewSetWithCapacity[string](len(routes))
	for index := range routes {
		route := &routes[index]
		if err := validateRoute(route, routeSet, refGraph); err != nil {
			return err
		}
	}
	return nil
}

func validateRoute(route *Route, routeSet *collectionset.Set[string], refGraph *collectiongraph.Graph[string, string]) error {
	if err := validateRouteIdentity(route, routeSet); err != nil {
		return err
	}
	routeNode := configNodeRoute + route.Name
	entrypointNode := configNodeEntrypoint + route.Entrypoint
	serviceNode := configNodeService + route.Service
	refGraph.AddNode(routeNode, "route")
	if err := validateRouteReferences(refGraph, route, entrypointNode, serviceNode); err != nil {
		return err
	}
	if err := addRouteBaseEdges(refGraph, routeNode, entrypointNode, serviceNode); err != nil {
		return err
	}
	if err := validateRouteMiddlewares(route, routeNode, refGraph); err != nil {
		return err
	}
	route.Method = strings.ToUpper(route.Method)
	routeSet.Add(route.Name)
	return nil
}

func validateRouteIdentity(route *Route, routeSet *collectionset.Set[string]) error {
	if route.Name == "" {
		return errors.New("route name cannot be empty")
	}
	if routeSet.Contains(route.Name) {
		return fmt.Errorf("duplicated route %q", route.Name)
	}
	return nil
}

func validateRouteReferences(
	refGraph *collectiongraph.Graph[string, string],
	route *Route,
	entrypointNode, serviceNode string,
) error {
	if !refGraph.HasNode(entrypointNode) {
		return fmt.Errorf("route %q references unknown entrypoint %q", route.Name, route.Entrypoint)
	}
	if !refGraph.HasNode(serviceNode) {
		return fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
	}
	return nil
}

func addRouteBaseEdges(refGraph *collectiongraph.Graph[string, string], routeNode, entrypointNode, serviceNode string) error {
	if err := addReferenceEdge(refGraph, routeNode, entrypointNode); err != nil {
		return err
	}
	return addReferenceEdge(refGraph, routeNode, serviceNode)
}

func validateRouteMiddlewares(route *Route, routeNode string, refGraph *collectiongraph.Graph[string, string]) error {
	for _, middleware := range route.Middlewares {
		middleware = strings.TrimSpace(middleware)
		if middleware == "" {
			continue
		}
		middlewareNode := configNodeMiddleware + middleware
		if !refGraph.HasNode(middlewareNode) {
			return fmt.Errorf("route %q references unknown middleware %q", route.Name, middleware)
		}
		if err := addReferenceEdge(refGraph, routeNode, middlewareNode); err != nil {
			return err
		}
	}
	return nil
}

func addReferenceEdge(refGraph *collectiongraph.Graph[string, string], from, to string) error {
	if err := refGraph.AddEdge(from, to); err != nil {
		return oops.
			In("config").
			With("from", from, "to", to).
			Wrapf(err, "add config reference")
	}
	return nil
}

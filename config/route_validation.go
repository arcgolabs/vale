package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	collectiongraph "github.com/arcgolabs/collectionx/graph"
	"github.com/arcgolabs/collectionx/mapping"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/oops"
)

func validateRoutes(routes []Route, refGraph *collectiongraph.Graph[string, string], entrypointProtocols *mapping.Map[string, string]) error {
	routeSet := collectionset.NewSetWithCapacity[string](len(routes))
	for index := range routes {
		route := &routes[index]
		if err := validateRoute(route, routeSet, refGraph, entrypointProtocols); err != nil {
			return err
		}
	}
	return nil
}

func validateRoute(route *Route, routeSet *collectionset.Set[string], refGraph *collectiongraph.Graph[string, string], entrypointProtocols *mapping.Map[string, string]) error {
	if err := validateRouteIdentity(route, routeSet); err != nil {
		return err
	}
	routeNode := configNodeRoute + route.Name
	entrypointNode := configNodeEntrypoint + route.Entrypoint
	serviceNode := configNodeService + route.Service
	refGraph.AddNode(routeNode, "route")
	if err := validateRouteReferences(refGraph, route, entrypointNode, serviceNode, entrypointProtocols); err != nil {
		return err
	}
	if err := addRouteBaseEdges(refGraph, routeNode, entrypointNode, serviceNode); err != nil {
		return err
	}
	if err := validateRouteMiddlewares(route, routeNode, refGraph); err != nil {
		return err
	}
	if err := validateRouteWriteTimeout(route); err != nil {
		return err
	}
	route.Method = strings.ToUpper(route.Method)
	routeSet.Add(route.Name)
	return nil
}

func validateRouteWriteTimeout(route *Route) error {
	raw := strings.TrimSpace(route.WriteTimeout)
	if raw == "" {
		return nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("route %q write_timeout %q is invalid: %w", route.Name, route.WriteTimeout, err)
	}
	if timeout < 0 {
		return fmt.Errorf("route %q write_timeout cannot be negative", route.Name)
	}
	route.WriteTimeout = raw
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
	entrypointProtocols *mapping.Map[string, string],
) error {
	if !refGraph.HasNode(entrypointNode) {
		return fmt.Errorf("route %q references unknown entrypoint %q", route.Name, route.Entrypoint)
	}
	if protocol, _ := entrypointProtocols.Get(route.Entrypoint); protocol == EntrypointProtocolTCP {
		return fmt.Errorf("route %q references tcp entrypoint %q", route.Name, route.Entrypoint)
	}
	if !refGraph.HasNode(serviceNode) {
		return fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
	}
	return nil
}

func validateTCPRoutes(routes []TCPRoute, refGraph *collectiongraph.Graph[string, string], entrypointProtocols *mapping.Map[string, string]) error {
	routeSet := collectionset.NewSetWithCapacity[string](len(routes))
	entrypointSet := collectionset.NewSetWithCapacity[string](len(routes))
	for index := range routes {
		route := &routes[index]
		if err := validateTCPRoute(route, routeSet, entrypointSet, refGraph, entrypointProtocols); err != nil {
			return err
		}
	}
	return nil
}

func validateTCPRoute(
	route *TCPRoute,
	routeSet *collectionset.Set[string],
	entrypointSet *collectionset.Set[string],
	refGraph *collectiongraph.Graph[string, string],
	entrypointProtocols *mapping.Map[string, string],
) error {
	if route.Name == "" {
		return errors.New("tcp_route name cannot be empty")
	}
	if routeSet.Contains(route.Name) {
		return fmt.Errorf("duplicated tcp_route %q", route.Name)
	}
	if entrypointSet.Contains(route.Entrypoint) {
		return fmt.Errorf("tcp entrypoint %q cannot have more than one tcp_route", route.Entrypoint)
	}
	entrypointNode := configNodeEntrypoint + route.Entrypoint
	serviceNode := configNodeTCPService + route.Service
	routeNode := configNodeTCPRoute + route.Name
	refGraph.AddNode(routeNode, "tcp_route")
	if !refGraph.HasNode(entrypointNode) {
		return fmt.Errorf("tcp_route %q references unknown entrypoint %q", route.Name, route.Entrypoint)
	}
	if protocol, _ := entrypointProtocols.Get(route.Entrypoint); protocol != EntrypointProtocolTCP {
		return fmt.Errorf("tcp_route %q references non-tcp entrypoint %q", route.Name, route.Entrypoint)
	}
	if !refGraph.HasNode(serviceNode) {
		return fmt.Errorf("tcp_route %q references unknown tcp_service %q", route.Name, route.Service)
	}
	if err := addRouteBaseEdges(refGraph, routeNode, entrypointNode, serviceNode); err != nil {
		return err
	}
	routeSet.Add(route.Name)
	entrypointSet.Add(route.Entrypoint)
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

package config

import (
	"fmt"
	"strings"

	collectiongraph "github.com/arcgolabs/collectionx/graph"
	collectionset "github.com/arcgolabs/collectionx/set"
)

const (
	configNodeEntrypoint = "entrypoint:"
	configNodeMiddleware = "middleware:"
	configNodeRoute      = "route:"
	configNodeService    = "service:"
)

func Validate(cfg *Config) error {
	if len(cfg.Entrypoints) == 0 {
		return fmt.Errorf("at least one entrypoint is required")
	}
	if len(cfg.Services) == 0 {
		return fmt.Errorf("at least one service is required")
	}
	if len(cfg.Routes) == 0 {
		return fmt.Errorf("at least one route is required")
	}
	entrypointSet := collectionset.NewSetWithCapacity[string](len(cfg.Entrypoints))
	for _, ep := range cfg.Entrypoints {
		if ep.Name == "" || ep.Address == "" {
			return fmt.Errorf("entrypoint name/address cannot be empty")
		}
		if ep.TLS != nil && ep.TLS.Enabled && (ep.TLS.CertFile == "") != (ep.TLS.KeyFile == "") {
			return fmt.Errorf("entrypoint %q tls requires both cert_file and key_file", ep.Name)
		}
		if ep.ACME != nil && ep.ACME.Enabled && len(ep.ACME.Domains) == 0 {
			return fmt.Errorf("entrypoint %q acme requires at least one domain", ep.Name)
		}
		if ep.ACME != nil && ep.ACME.Enabled && strings.TrimSpace(ep.ACME.Email) == "" {
			return fmt.Errorf("entrypoint %q acme requires email", ep.Name)
		}
		if entrypointSet.Contains(ep.Name) {
			return fmt.Errorf("duplicated entrypoint %q", ep.Name)
		}
		entrypointSet.Add(ep.Name)
	}

	serviceSet := collectionset.NewSetWithCapacity[string](len(cfg.Services))
	for _, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("service name cannot be empty")
		}
		if serviceSet.Contains(svc.Name) {
			return fmt.Errorf("duplicated service %q", svc.Name)
		}
		if len(svc.Endpoints) == 0 {
			return fmt.Errorf("service %q must have at least one endpoint", svc.Name)
		}
		for _, endpoint := range svc.Endpoints {
			if endpoint.URL == "" {
				return fmt.Errorf("service %q contains empty endpoint url", svc.Name)
			}
		}
		serviceSet.Add(svc.Name)
	}

	middlewareSet := collectionset.NewSetWithCapacity[string](len(cfg.Middlewares))
	for _, middleware := range cfg.Middlewares {
		if middleware.Name == "" {
			return fmt.Errorf("middleware name cannot be empty")
		}
		if middlewareSet.Contains(middleware.Name) {
			return fmt.Errorf("duplicated middleware %q", middleware.Name)
		}
		middlewareSet.Add(middleware.Name)
	}

	routeSet := collectionset.NewSetWithCapacity[string](len(cfg.Routes))
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
	for _, route := range cfg.Routes {
		if route.Name == "" {
			return fmt.Errorf("route name cannot be empty")
		}
		if routeSet.Contains(route.Name) {
			return fmt.Errorf("duplicated route %q", route.Name)
		}
		routeNode := configNodeRoute + route.Name
		entrypointNode := configNodeEntrypoint + route.Entrypoint
		serviceNode := configNodeService + route.Service
		refGraph.AddNode(routeNode, "route")
		if !refGraph.HasNode(entrypointNode) {
			return fmt.Errorf("route %q references unknown entrypoint %q", route.Name, route.Entrypoint)
		}
		if !refGraph.HasNode(serviceNode) {
			return fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
		}
		_ = refGraph.AddEdge(routeNode, entrypointNode)
		_ = refGraph.AddEdge(routeNode, serviceNode)
		for _, middleware := range route.Middlewares {
			middlewareNode := configNodeMiddleware + middleware
			if !refGraph.HasNode(middlewareNode) {
				return fmt.Errorf("route %q references unknown middleware %q", route.Name, middleware)
			}
			_ = refGraph.AddEdge(routeNode, middlewareNode)
		}
		route.Method = strings.ToUpper(route.Method)
		routeSet.Add(route.Name)
	}

	return nil
}

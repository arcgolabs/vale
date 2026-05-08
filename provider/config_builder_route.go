package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/config"
)

func (b *ConfigBuilder) RouteTo(name, entrypoint, service string, options ...RouteOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	route := config.Route{
		Name:       strings.TrimSpace(name),
		Entrypoint: strings.TrimSpace(entrypoint),
		Service:    strings.TrimSpace(service),
		Headers:    map[string]string{},
	}
	b.validateRoute(route)
	collectionlist.NewList(options...).Range(func(_ int, option RouteOption) bool {
		if option != nil {
			option(&route)
		}
		return true
	})
	b.routes.Add(route)
	return b
}

func (b *ConfigBuilder) Route(route config.Route) *ConfigBuilder {
	if b == nil {
		return nil
	}
	if strings.TrimSpace(route.Name) == "" {
		b.addError("route name cannot be empty")
	}
	b.routes.Add(route)
	return b
}

func RouteHost(host string) RouteOption {
	return func(route *config.Route) {
		if route != nil {
			route.Host = strings.TrimSpace(host)
		}
	}
}

func RoutePathPrefix(pathPrefix string) RouteOption {
	return func(route *config.Route) {
		if route != nil {
			route.PathPrefix = strings.TrimSpace(pathPrefix)
		}
	}
}

func RouteMethod(method string) RouteOption {
	return func(route *config.Route) {
		if route != nil {
			route.Method = strings.TrimSpace(method)
		}
	}
}

func RouteHeader(key, value string) RouteOption {
	return func(route *config.Route) {
		if route == nil {
			return
		}
		if route.Headers == nil {
			route.Headers = map[string]string{}
		}
		route.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func RouteMiddlewares(names ...string) RouteOption {
	return func(route *config.Route) {
		if route == nil {
			return
		}
		route.Middlewares = cleanStrings(collectionlist.NewList(names...)).Values()
	}
}

func (b *ConfigBuilder) validateRoute(route config.Route) {
	if route.Name == "" {
		b.addError("route name cannot be empty")
	}
	if route.Entrypoint == "" {
		b.addError("route %q entrypoint cannot be empty", route.Name)
	}
	if route.Service == "" {
		b.addError("route %q service cannot be empty", route.Name)
	}
}

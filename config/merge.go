package config

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

func Merge(configs ...*Config) *Config {
	return MergeList(collectionlist.NewList(configs...))
}

func MergeList(configs *collectionlist.List[*Config]) *Config {
	merged := &Config{}
	entrypoints := mapping.NewOrderedMap[string, Entrypoint]()
	services := mapping.NewOrderedMap[string, Service]()
	tcpServices := mapping.NewOrderedMap[string, TCPService]()
	middlewares := mapping.NewOrderedMap[string, Middleware]()
	routes := mapping.NewOrderedMap[string, Route]()
	tcpRoutes := mapping.NewOrderedMap[string, TCPRoute]()

	configs.Range(func(_ int, cfg *Config) bool {
		if cfg == nil {
			return true
		}
		mergeConfigResources(cfg, entrypoints, services, tcpServices, middlewares, routes, tcpRoutes)
		mergeConfigBlocks(merged, cfg)
		return true
	})

	merged.Entrypoints = entrypoints.Values()
	merged.Services = services.Values()
	merged.TCPServices = tcpServices.Values()
	merged.Middlewares = middlewares.Values()
	merged.Routes = routes.Values()
	merged.TCPRoutes = tcpRoutes.Values()
	return merged
}

func mergeConfigResources(
	cfg *Config,
	entrypoints *mapping.OrderedMap[string, Entrypoint],
	services *mapping.OrderedMap[string, Service],
	tcpServices *mapping.OrderedMap[string, TCPService],
	middlewares *mapping.OrderedMap[string, Middleware],
	routes *mapping.OrderedMap[string, Route],
	tcpRoutes *mapping.OrderedMap[string, TCPRoute],
) {
	for _, entrypoint := range cfg.Entrypoints {
		entrypoints.Set(entrypoint.Name, entrypoint)
	}
	for _, service := range cfg.Services {
		services.Set(service.Name, service)
	}
	for _, service := range cfg.TCPServices {
		tcpServices.Set(service.Name, service)
	}
	for index := range cfg.Middlewares {
		middleware := cfg.Middlewares[index]
		middlewares.Set(middleware.Name, middleware)
	}
	for index := range cfg.Routes {
		route := cfg.Routes[index]
		routes.Set(route.Name, route)
	}
	for index := range cfg.TCPRoutes {
		route := cfg.TCPRoutes[index]
		tcpRoutes.Set(route.Name, route)
	}
}

func mergeConfigBlocks(merged, cfg *Config) {
	if cfg.Admin != nil {
		admin := *cfg.Admin
		merged.Admin = &admin
	}
	if cfg.Observability != nil {
		observability := *cfg.Observability
		merged.Observability = &observability
	}
	if cfg.Health != nil {
		health := *cfg.Health
		merged.Health = &health
	}
	if cfg.Security != nil {
		security := *cfg.Security
		merged.Security = &security
	}
}

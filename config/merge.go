package config

import "github.com/arcgolabs/collectionx/mapping"

func Merge(configs ...*Config) *Config {
	merged := &Config{}
	entrypoints := mapping.NewOrderedMap[string, Entrypoint]()
	services := mapping.NewOrderedMap[string, Service]()
	middlewares := mapping.NewOrderedMap[string, Middleware]()
	routes := mapping.NewOrderedMap[string, Route]()

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		for _, entrypoint := range cfg.Entrypoints {
			entrypoints.Set(entrypoint.Name, entrypoint)
		}

		for _, service := range cfg.Services {
			services.Set(service.Name, service)
		}

		for _, middleware := range cfg.Middlewares {
			middlewares.Set(middleware.Name, middleware)
		}

		for _, route := range cfg.Routes {
			routes.Set(route.Name, route)
		}
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

	merged.Entrypoints = entrypoints.Values()
	merged.Services = services.Values()
	merged.Middlewares = middlewares.Values()
	merged.Routes = routes.Values()
	return merged
}

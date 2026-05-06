package provider

import (
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
)

func NewEntrypointConfig(name string, address string) *config.Config {
	return &config.Config{
		Entrypoints: []config.Entrypoint{
			{
				Name:    name,
				Address: address,
			},
		},
		Services: make([]config.Service, 0),
		Routes:   make([]config.Route, 0),
	}
}

func AppendSortedServices(cfg *config.Config, services *mapping.Map[string, *config.Service]) {
	if cfg == nil || services == nil {
		return
	}
	for _, serviceName := range SortedStrings(services.Keys()) {
		service, _ := services.Get(serviceName)
		if service != nil {
			cfg.Services = append(cfg.Services, *service)
		}
	}
}

func AppendSortedRoutes(cfg *config.Config, routes *mapping.Map[string, config.Route]) {
	if cfg == nil || routes == nil {
		return
	}
	for _, routeName := range SortedStrings(routes.Keys()) {
		route, _ := routes.Get(routeName)
		cfg.Routes = append(cfg.Routes, route)
	}
}

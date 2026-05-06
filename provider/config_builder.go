package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
)

type ConfigBuilder struct {
	entrypoints *collectionlist.List[config.Entrypoint]
	services    *collectionlist.List[config.Service]
	middlewares *collectionlist.List[config.Middleware]
	routes      *collectionlist.List[config.Route]
	cfg         *config.Config
}

func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		entrypoints: collectionlist.NewList[config.Entrypoint](),
		services:    collectionlist.NewList[config.Service](),
		middlewares: collectionlist.NewList[config.Middleware](),
		routes:      collectionlist.NewList[config.Route](),
		cfg:         &config.Config{},
	}
}

func (b *ConfigBuilder) Entrypoint(name string, address string) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.entrypoints.Add(config.Entrypoint{Name: strings.TrimSpace(name), Address: strings.TrimSpace(address)})
	return b
}

func (b *ConfigBuilder) Service(name string, endpointURL string) *ConfigBuilder {
	return b.ServiceWithEndpoints(name, config.Endpoint{URL: endpointURL, Weight: 1})
}

func (b *ConfigBuilder) ServiceWithEndpoints(name string, endpoints ...config.Endpoint) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.services.Add(config.Service{
		Name:      strings.TrimSpace(name),
		Strategy:  "round_robin",
		Endpoints: collectionlist.NewList(endpoints...).Values(),
	})
	return b
}

func (b *ConfigBuilder) Middleware(middleware config.Middleware) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.middlewares.Add(middleware)
	return b
}

func (b *ConfigBuilder) Route(route config.Route) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.routes.Add(route)
	return b
}

func (b *ConfigBuilder) Admin(address string) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Admin = &config.Admin{Address: strings.TrimSpace(address)}
	return b
}

func (b *ConfigBuilder) Build() *config.Config {
	if b == nil {
		return &config.Config{}
	}
	cfg := *b.cfg
	cfg.Entrypoints = b.entrypoints.Values()
	cfg.Services = b.services.Values()
	cfg.Middlewares = b.middlewares.Values()
	cfg.Routes = b.routes.Values()
	return &cfg
}

func NewEntrypointConfig(name string, address string) *config.Config {
	return &config.Config{
		Entrypoints: []config.Entrypoint{
			{
				Name:    name,
				Address: address,
			},
		},
		Middlewares: make([]config.Middleware, 0),
		Services:    make([]config.Service, 0),
		Routes:      make([]config.Route, 0),
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

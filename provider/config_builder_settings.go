package provider

import (
	"strings"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
)

func (b *ConfigBuilder) Admin(address string) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Admin = &config.Admin{Address: strings.TrimSpace(address)}
	return b
}

func (b *ConfigBuilder) Observability(accessLog, metrics bool) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Observability = &config.Observability{AccessLog: accessLog, Metrics: metrics}
	return b
}

func (b *ConfigBuilder) Health(interval, timeout string) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Health = &config.Health{
		Interval: strings.TrimSpace(interval),
		Timeout:  strings.TrimSpace(timeout),
	}
	return b
}

func (b *ConfigBuilder) Security(security config.Security) *ConfigBuilder {
	if b == nil {
		return nil
	}
	b.cfg.Security = &security
	return b
}

func NewEntrypointConfig(name, address string) *config.Config {
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

package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
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
	sortedServices := collectionlist.FilterMapList(SortedStrings(collectionlist.NewList(services.Keys()...)), func(_ int, serviceName string) (config.Service, bool) {
		service, _ := services.Get(serviceName)
		if service != nil {
			return *service, true
		}
		return config.Service{}, false
	})
	cfg.Services = collectionlist.NewList(cfg.Services...).Merge(sortedServices).Values()
}

func AppendSortedRoutes(cfg *config.Config, routes *mapping.Map[string, config.Route]) {
	if cfg == nil || routes == nil {
		return
	}
	sortedRoutes := collectionlist.MapList(SortedStrings(collectionlist.NewList(routes.Keys()...)), func(_ int, routeName string) config.Route {
		route, _ := routes.Get(routeName)
		return route
	})
	cfg.Routes = collectionlist.NewList(cfg.Routes...).Merge(sortedRoutes).Values()
}

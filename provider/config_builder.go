package provider

import (
	"errors"
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/config"
)

type EntrypointOption func(*config.Entrypoint)
type RouteOption func(*config.Route)
type MiddlewareOption func(*config.Middleware)

type ConfigBuilder struct {
	entrypoints *collectionlist.List[config.Entrypoint]
	services    *collectionlist.List[config.Service]
	middlewares *collectionlist.List[config.Middleware]
	routes      *collectionlist.List[config.Route]
	cfg         *config.Config
	errors      *collectionlist.List[error]
}

func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		entrypoints: collectionlist.NewList[config.Entrypoint](),
		services:    collectionlist.NewList[config.Service](),
		middlewares: collectionlist.NewList[config.Middleware](),
		routes:      collectionlist.NewList[config.Route](),
		cfg:         &config.Config{},
		errors:      collectionlist.NewList[error](),
	}
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

func (b *ConfigBuilder) BuildValidated() (*config.Config, error) {
	if b == nil {
		return &config.Config{}, errors.New("config builder is nil")
	}
	cfg := b.Build()
	errs := collectionlist.NewList[error]()
	errs.Merge(b.errors)
	if err := config.Validate(cfg); err != nil {
		errs.Add(err)
	}
	return cfg, errors.Join(errs.Values()...)
}

func (b *ConfigBuilder) Errors() *collectionlist.List[error] {
	if b == nil || b.errors == nil {
		return collectionlist.NewList[error]()
	}
	return b.errors.Clone()
}

func (b *ConfigBuilder) addError(format string, args ...any) {
	if b == nil {
		return
	}
	if b.errors == nil {
		b.errors = collectionlist.NewList[error]()
	}
	b.errors.Add(fmt.Errorf(format, args...))
}

package memoryconfig

import (
	"context"
	"errors"
	"io"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/samber/oops"
)

type Provider struct {
	name string

	cfgState *provider.StateStore[*config.Config]
}

func New(name string, cfg *config.Config) (*Provider, error) {
	if cfg == nil {
		return nil, errors.New("memory config provider: config cannot be nil")
	}
	if err := config.Validate(cfg); err != nil {
		return nil, oops.In("provider.memoryconfig").Wrapf(err, "validate memory config")
	}
	if name == "" {
		name = "memory-config"
	}
	return &Provider{
		name:     name,
		cfgState: provider.NewStateStore(cfg, nil),
	}, nil
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(_ context.Context) (*config.Config, error) {
	return p.cfgState.Load(), nil
}

func (p *Provider) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	return p.cfgState.Watch(onReload), nil
}

func (p *Provider) Update(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("memory config provider: config cannot be nil")
	}
	if err := config.Validate(cfg); err != nil {
		return oops.In("provider.memoryconfig").Wrapf(err, "validate memory config")
	}

	p.cfgState.Update(cfg)
	return nil
}

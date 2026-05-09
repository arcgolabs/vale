package memoryconfig

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/samber/oops"
)

type Provider struct {
	name string

	mu       sync.RWMutex
	cfg      *config.Config
	watchHub *provider.WatchHub
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
		cfg:      cfg,
		watchHub: provider.NewWatchHub(),
	}, nil
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(_ context.Context) (*config.Config, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg, nil
}

func (p *Provider) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	return p.watchHub.Watch(onReload), nil
}

func (p *Provider) Update(cfg *config.Config) error {
	if cfg == nil {
		return errors.New("memory config provider: config cannot be nil")
	}
	if err := config.Validate(cfg); err != nil {
		return oops.In("provider.memoryconfig").Wrapf(err, "validate memory config")
	}

	p.mu.Lock()
	p.cfg = cfg
	p.mu.Unlock()

	p.watchHub.Notify()
	return nil
}

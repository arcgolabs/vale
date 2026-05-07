package memoryconfig

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	"github.com/samber/oops"
)

type Provider struct {
	name string

	mu        sync.RWMutex
	cfg       *config.Config
	listeners *mapping.Map[int, func()]
	nextID    int
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
		name:      name,
		cfg:       cfg,
		listeners: mapping.NewMap[int, func()](),
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
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.nextID
	p.nextID++
	p.listeners.Set(id, onReload)
	return provider.NewOnceCloser(func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.listeners.Delete(id)
	}), nil
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
	listeners := p.listeners.Values()
	p.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
	return nil
}

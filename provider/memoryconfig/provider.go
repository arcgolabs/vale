package memoryconfig

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/arcgolabs/vela/config"
)

type Provider struct {
	name string

	mu        sync.RWMutex
	cfg       *config.Config
	listeners map[int]func()
	nextID    int
}

func New(name string, cfg *config.Config) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("memory config provider: config cannot be nil")
	}
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}
	if name == "" {
		name = "memory-config"
	}
	return &Provider{
		name:      name,
		cfg:       cfg,
		listeners: make(map[int]func()),
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
	p.listeners[id] = onReload
	return &watchCloser{
		closeFn: func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			delete(p.listeners, id)
		},
	}, nil
}

func (p *Provider) Update(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("memory config provider: config cannot be nil")
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}

	p.mu.Lock()
	p.cfg = cfg
	listeners := make([]func(), 0, len(p.listeners))
	for _, listener := range p.listeners {
		listeners = append(listeners, listener)
	}
	p.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
	return nil
}

type watchCloser struct {
	once    sync.Once
	closeFn func()
}

func (c *watchCloser) Close() error {
	c.once.Do(func() {
		if c.closeFn != nil {
			c.closeFn()
		}
	})
	return nil
}

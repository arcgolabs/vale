package staticconfig

import (
	"context"
	"io"

	"github.com/arcgolabs/vela/config"
)

type Provider struct {
	name string
	cfg  *config.Config
}

func New(cfg *config.Config) *Provider {
	return &Provider{
		name: "static-config",
		cfg:  cfg,
	}
}

func NewNamed(name string, cfg *config.Config) *Provider {
	if name == "" {
		name = "static-config"
	}
	return &Provider{
		name: name,
		cfg:  cfg,
	}
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(_ context.Context) (*config.Config, error) {
	return p.cfg, nil
}

func (p *Provider) Watch(_ context.Context, _ func(), _ func(error)) (io.Closer, error) {
	return nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error {
	return nil
}

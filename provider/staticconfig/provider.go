package staticconfig

import (
	"context"
	"io"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
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
	return provider.NopCloser{}, nil
}

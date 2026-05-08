// Package docker provides a Docker label config provider for Vela.
package docker

import (
	"context"
	"errors"
	"io"
	"log/slog"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
	"github.com/samber/oops"
)

type Container struct {
	Name    string
	Address string
	Port    int
	Labels  *mapping.Map[string, string]
}

type Source interface {
	ListContainers(context.Context) (*collectionlist.List[Container], error)
	Watch(context.Context, func(), func(error)) (io.Closer, error)
}

type Provider struct {
	name    string
	source  Source
	options Options
	logger  *slog.Logger
}

type Options struct {
	DefaultEntrypointName string
	DefaultEntrypointAddr string
	EntrypointAddresses   *mapping.Map[string, string]
}

func DefaultOptions() Options {
	entrypointAddresses := mapping.NewMap[string, string]()
	entrypointAddresses.Set("web", ":8080")
	return Options{
		DefaultEntrypointName: "web",
		DefaultEntrypointAddr: ":8080",
		EntrypointAddresses:   entrypointAddresses,
	}
}

func New(name string, source Source, options Options) *Provider {
	if name == "" {
		name = "docker"
	}
	options = normalizeOptions(options)
	return &Provider{
		name:    name,
		source:  source,
		options: options,
	}
}

func (p *Provider) SetLogger(logger *slog.Logger) {
	p.logger = logger
}

func NewFromEnv(name string, options Options) (*Provider, error) {
	source, err := NewDockerSourceFromEnv()
	if err != nil {
		return nil, err
	}
	return New(name, source, options), nil
}

func (p *Provider) Name() string {
	return p.name
}

func (p *Provider) Load(ctx context.Context) (*config.Config, error) {
	if p.source == nil {
		return nil, errors.New("docker provider source is nil")
	}
	containers, err := p.source.ListContainers(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("docker source list failed", "provider", p.name, "error", err)
		}
		return nil, oops.In("docker_provider").Wrapf(err, "list docker containers")
	}
	p.logContainersListed(containers)

	result := buildConfig(p.options, containers)
	if err := config.Validate(result.Config); err != nil {
		if p.logger != nil {
			p.logger.Error("docker config validation failed", "provider", p.name, "error", err)
		}
		return nil, oops.In("docker_provider").Wrapf(err, "validate docker config")
	}
	p.logConfigBuilt(containers, result)
	return result.Config, nil
}

func (p *Provider) Watch(ctx context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	if p.source == nil {
		return nil, errors.New("docker provider source is nil")
	}
	closer, err := p.source.Watch(ctx, onReload, onError)
	if err != nil {
		return nil, oops.In("docker_provider").Wrapf(err, "watch docker source")
	}
	return closer, nil
}

func normalizeOptions(options Options) Options {
	defaults := DefaultOptions()
	if options.DefaultEntrypointName == "" {
		options.DefaultEntrypointName = defaults.DefaultEntrypointName
	}
	if options.DefaultEntrypointAddr == "" {
		options.DefaultEntrypointAddr = defaults.DefaultEntrypointAddr
	}
	if options.EntrypointAddresses == nil {
		options.EntrypointAddresses = mapping.NewMap[string, string]()
	}
	if _, ok := options.EntrypointAddresses.Get(options.DefaultEntrypointName); !ok {
		options.EntrypointAddresses.Set(options.DefaultEntrypointName, options.DefaultEntrypointAddr)
	}
	return options
}

func (p *Provider) logContainersListed(containers *collectionlist.List[Container]) {
	if p.logger != nil {
		p.logger.Info("docker containers listed", "provider", p.name, "containers", containers.Len())
	}
}

func (p *Provider) logConfigBuilt(containers *collectionlist.List[Container], result configBuildResult) {
	if p.logger == nil {
		return
	}
	p.logger.Info("docker config built",
		"provider", p.name,
		"containers", containers.Len(),
		"disabled", result.DisabledCount,
		"invalid_endpoints", result.InvalidEndpointCount,
		"middlewares", len(result.Config.Middlewares),
		"services", len(result.Config.Services),
		"routes", len(result.Config.Routes),
	)
}

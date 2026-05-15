// Package vale is the library-first entrypoint for embedding Vale.
//
// The root package intentionally exposes process-independent construction APIs.
// The standalone valed binary wires the same components with dix/configx/logx
// under cmd/.
package vale

import (
	"log/slog"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/gateway"
	"github.com/arcgolabs/vale/provider"
	staticconfigprovider "github.com/arcgolabs/vale/provider/staticconfig"
	"github.com/arcgolabs/vale/runtime"
	"github.com/samber/oops"
)

type (
	Gateway     = gateway.Gateway
	Config      = gateway.Config
	Option      = gateway.Option
	Cluster     = gateway.Cluster
	ClusterPeer = gateway.ClusterPeer
)

func New(options ...Option) (*Gateway, error) {
	return NewGatewayBuilder(GatewayOptions(options...)).Build()
}

func NewFromConfig(cfg Config) (*Gateway, error) {
	applyDefaultConfigSource(&cfg)
	gw, err := gateway.NewFromConfig(cfg)
	if err != nil {
		return nil, oops.
			In("vale").
			Wrapf(err, "create gateway from config")
	}
	return gw, nil
}

func NewDefault() (*Gateway, error) {
	return New()
}

func MustNew(options ...Option) *Gateway {
	gw, err := New(options...)
	if err != nil {
		panic(err)
	}
	return gw
}

func DefaultConfig() Config {
	return gateway.DefaultConfig()
}

func WithWatch(enabled bool) Option {
	return gateway.WithWatch(enabled)
}

func WithClusterFactory(factory gateway.ClusterFactory) Option {
	return gateway.WithClusterFactory(factory)
}

func WithCertificateStorage(storage certstore.Storage) Option {
	return gateway.WithCertificateStorage(storage)
}

func WithMetricsFactory(factory gateway.MetricsFactory) Option {
	return gateway.WithMetricsFactory(factory)
}

func WithMiddlewareRegistry(registry *runtime.MiddlewareRegistry) Option {
	return gateway.WithMiddlewareRegistry(registry)
}

func WithObservability(obs observabilityx.Observability) Option {
	return gateway.WithObservability(obs)
}

func WithLogger(logger *slog.Logger) Option {
	return gateway.WithLogger(logger)
}

func WithEventBus(bus provider.EventBus) Option {
	return gateway.WithEventBus(bus)
}

func WithSnapshotProvider(snapshotProvider provider.SnapshotProvider) Option {
	return gateway.WithSnapshotProvider(snapshotProvider)
}

func WithConfigSourceProviders(configProviders ...provider.ConfigProvider) Option {
	return gateway.WithConfigSourceProviders(configProviders...)
}

func WithStaticConfig(cfgData *config.Config) Option {
	return gateway.WithStaticConfig(cfgData)
}

func WithFallbackProviders(providers ...provider.SnapshotProvider) Option {
	return gateway.WithFallbackProviders(providers...)
}

func WithStaticSnapshot(snapshot *runtime.CompiledSnapshot) Option {
	return gateway.WithStaticSnapshot(snapshot)
}

func WithWatchErrorHandler(handler func(error)) Option {
	return gateway.WithWatchErrorHandler(handler)
}

func applyDefaultConfigSource(cfg *Config) {
	if cfg == nil {
		return
	}
	if cfg.Provider != nil || !cfg.ConfigSource.IsEmpty() {
		return
	}
	cfg.ConfigSource = collectionlist.NewList[provider.ConfigProvider](staticconfigprovider.New(config.Default()))
	cfg.Watch = false
}

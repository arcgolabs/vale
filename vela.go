// Package vela is the library-first entrypoint for embedding Vela.
//
// The root package intentionally exposes process-independent construction APIs.
// The standalone velad binary wires the same components with dix/configx/logx
// under cmd/.
package vela

import (
	"log/slog"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/provider"
	staticconfigprovider "github.com/arcgolabs/vela/provider/staticconfig"
	"github.com/arcgolabs/vela/runtime"
)

type (
	Gateway = gateway.Gateway
	Config  = gateway.Config
	Option  = gateway.Option
	Cluster = gateway.Cluster
)

func New(options ...Option) (*Gateway, error) {
	cfg := DefaultConfig()
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, err
		}
	}
	applyDefaultConfigSource(&cfg)
	return gateway.NewFromConfig(cfg)
}

func NewFromConfig(cfg Config) (*Gateway, error) {
	applyDefaultConfigSource(&cfg)
	return gateway.NewFromConfig(cfg)
}

func NewDefault() (*Gateway, error) {
	return New()
}

func MustNew(options ...Option) *Gateway {
	gateway, err := New(options...)
	if err != nil {
		panic(err)
	}
	return gateway
}

func DefaultConfig() Config {
	cfg := gateway.DefaultConfig()
	cfg.ConfigPath = ""
	return cfg
}

func WithConfigPath(path string) Option {
	return gateway.WithConfigPath(path)
}

func WithConfigFiles(paths ...string) Option {
	return gateway.WithConfigFiles(paths...)
}

func WithWatch(enabled bool) Option {
	return gateway.WithWatch(enabled)
}

func WithClusterFactory(factory gateway.ClusterFactory) Option {
	return gateway.WithClusterFactory(factory)
}

func WithLogger(logger *slog.Logger) Option {
	return gateway.WithLogger(logger)
}

func WithEventBus(bus eventx.BusRuntime) Option {
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
	if cfg.Provider != nil || len(cfg.ConfigSource) > 0 || cfg.ConfigPath != "" || len(cfg.ConfigFiles) > 0 {
		return
	}
	cfg.ConfigSource = []provider.ConfigProvider{staticconfigprovider.New(config.Default())}
	cfg.Watch = false
}

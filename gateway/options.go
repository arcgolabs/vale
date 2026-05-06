package gateway

import (
	"fmt"
	"log/slog"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	staticprovider "github.com/arcgolabs/vela/provider/static"
	staticconfigprovider "github.com/arcgolabs/vela/provider/staticconfig"
	"github.com/arcgolabs/vela/runtime"
	"github.com/samber/lo"
)

// Option configures [Config] when passed to [New]. Return a non-nil error from a custom
// Option to fail construction.
type Option func(*Config) error

func WithWatch(enabled bool) Option {
	return func(cfg *Config) error {
		cfg.Watch = enabled
		return nil
	}
}

func WithClusterFactory(factory ClusterFactory) Option {
	return func(cfg *Config) error {
		if factory == nil {
			return fmt.Errorf("cluster factory cannot be nil")
		}
		cfg.Cluster = factory
		return nil
	}
}

func WithMetricsFactory(factory MetricsFactory) Option {
	return func(cfg *Config) error {
		if factory == nil {
			return fmt.Errorf("metrics factory cannot be nil")
		}
		cfg.Metrics = factory
		return nil
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(cfg *Config) error {
		cfg.Logger = logger
		return nil
	}
}

func WithEventBus(bus provider.EventBus) Option {
	return func(cfg *Config) error {
		if bus == nil {
			return fmt.Errorf("event bus cannot be nil")
		}
		cfg.EventBus = bus
		return nil
	}
}

func WithSnapshotProvider(snapshotProvider provider.SnapshotProvider) Option {
	return func(cfg *Config) error {
		if snapshotProvider == nil {
			return fmt.Errorf("snapshot provider cannot be nil")
		}
		cfg.Provider = snapshotProvider
		cfg.ConfigSource = nil
		return nil
	}
}

func WithConfigSourceProviders(configProviders ...provider.ConfigProvider) Option {
	return func(cfg *Config) error {
		nonNil := lo.Filter(configProviders, func(p provider.ConfigProvider, _ int) bool {
			return p != nil
		})
		if len(nonNil) == 0 {
			return fmt.Errorf("config source providers cannot be empty")
		}
		cfg.ConfigSource = nonNil
		cfg.Provider = nil
		return nil
	}
}

func WithStaticConfig(cfgData *config.Config) Option {
	return func(cfg *Config) error {
		if cfgData == nil {
			return fmt.Errorf("static config cannot be nil")
		}
		if err := config.Validate(cfgData); err != nil {
			return err
		}
		cfg.ConfigSource = []provider.ConfigProvider{staticconfigprovider.New(cfgData)}
		cfg.Provider = nil
		cfg.Watch = false
		return nil
	}
}

func WithFallbackProviders(providers ...provider.SnapshotProvider) Option {
	return func(cfg *Config) error {
		nonNil := lo.Filter(providers, func(p provider.SnapshotProvider, _ int) bool {
			return p != nil
		})
		if len(nonNil) == 0 {
			return fmt.Errorf("fallback providers cannot be empty")
		}
		cfg.Provider = provider.Fallback(nonNil...)
		cfg.ConfigSource = nil
		return nil
	}
}

func WithStaticSnapshot(snapshot *runtime.CompiledSnapshot) Option {
	return func(cfg *Config) error {
		if snapshot == nil {
			return fmt.Errorf("static snapshot cannot be nil")
		}
		cfg.Provider = staticprovider.New(snapshot)
		cfg.ConfigSource = nil
		cfg.Watch = false
		return nil
	}
}

func WithWatchErrorHandler(handler func(error)) Option {
	return func(cfg *Config) error {
		cfg.OnWatchError = handler
		return nil
	}
}

package gateway

import (
	"fmt"
	"log/slog"

	"github.com/arcgolabs/eventx"
	raftnode "github.com/arcgolabs/vela/cluster/raftnode"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	staticprovider "github.com/arcgolabs/vela/provider/static"
	staticconfigprovider "github.com/arcgolabs/vela/provider/staticconfig"
	"github.com/arcgolabs/vela/runtime"
)

// Option configures [Config] when passed to [New]. Return a non-nil error from a custom
// Option to fail construction.
type Option func(*Config) error

func WithConfigPath(path string) Option {
	return func(cfg *Config) error {
		if path == "" {
			return fmt.Errorf("config path cannot be empty")
		}
		cfg.ConfigPath = path
		cfg.ConfigFiles = nil
		return nil
	}
}

func WithConfigFiles(paths ...string) Option {
	return func(cfg *Config) error {
		if len(paths) == 0 {
			return fmt.Errorf("config files cannot be empty")
		}
		nonEmpty := make([]string, 0, len(paths))
		for _, path := range paths {
			if path == "" {
				return fmt.Errorf("config file path cannot be empty")
			}
			nonEmpty = append(nonEmpty, path)
		}
		cfg.ConfigFiles = nonEmpty
		return nil
	}
}

func WithWatch(enabled bool) Option {
	return func(cfg *Config) error {
		cfg.Watch = enabled
		return nil
	}
}

func WithRaftCluster(cluster raftnode.Config) Option {
	return func(cfg *Config) error {
		clusterConfig := cluster
		cfg.Cluster = &clusterConfig
		return nil
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(cfg *Config) error {
		cfg.Logger = logger
		return nil
	}
}

func WithEventBus(bus eventx.BusRuntime) Option {
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
		nonNil := make([]provider.ConfigProvider, 0, len(configProviders))
		for _, p := range configProviders {
			if p != nil {
				nonNil = append(nonNil, p)
			}
		}
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
		nonNil := make([]provider.SnapshotProvider, 0, len(providers))
		for _, p := range providers {
			if p != nil {
				nonNil = append(nonNil, p)
			}
		}
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

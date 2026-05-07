package main

import (
	"log/slog"
	"strings"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/arcgolabs/vela"
	raftnode "github.com/arcgolabs/vela/cluster/raftnode"
	prometheusmetrics "github.com/arcgolabs/vela/observability/prometheus"
	fileconfig "github.com/arcgolabs/vela/provider/fileconfig"
	"github.com/samber/oops"
)

const defaultMetricsName = "prometheus"

type (
	veladBaseOptions         []vela.Option
	veladMetricsOptions      []vela.Option
	veladConfigSourceOptions []vela.Option
	veladClusterOptions      []vela.Option
	veladEventBusOptions     []vela.Option
	veladGatewayOptions      []vela.Option
)

func provideLogger(cfg veladConfig) (*slog.Logger, error) {
	logger, err := logx.New(
		logx.WithConsole(true),
		logx.WithLevelString(cfg.LogLevel),
		logx.WithGlobalLogger(),
	)
	if err != nil {
		return nil, oops.
			In("cmd").
			With("log_level", cfg.LogLevel).
			Wrapf(err, "create logger")
	}
	logx.SetDefault(logger)
	logger.Info("logger configured", "level", cfg.LogLevel)
	return logger, nil
}

func provideEventBus() eventx.BusRuntime {
	return eventx.New()
}

func providePluginRegistry() (*vela.Registry, error) {
	registry := vela.NewRegistry()
	if err := registry.RegisterMetricsFactory(defaultMetricsName, prometheusmetrics.New); err != nil {
		return nil, oops.
			In("cmd").
			With("metrics", defaultMetricsName).
			Wrapf(err, "register metrics factory")
	}
	return registry, nil
}

func provideBaseOptions(cfg veladConfig, logger *slog.Logger) veladBaseOptions {
	return veladBaseOptions{
		vela.WithWatch(cfg.Watch),
		vela.WithLogger(logger),
	}
}

func provideMetricsOptions(registry *vela.Registry) veladMetricsOptions {
	return veladMetricsOptions{
		vela.WithMetricsFromRegistry(registry, defaultMetricsName),
	}
}

func provideConfigSourceOptions(cfg veladConfig) veladConfigSourceOptions {
	files := parseCSV(cfg.ConfigFiles)
	switch {
	case len(files) > 0:
		return veladConfigSourceOptions{fileconfig.WithConfigFiles(files...)}
	case strings.TrimSpace(cfg.ConfigPath) != "":
		return veladConfigSourceOptions{fileconfig.WithConfigPath(cfg.ConfigPath)}
	default:
		return nil
	}
}

func provideClusterOptions(cfg veladConfig) veladClusterOptions {
	if !cfg.RaftEnabled {
		return nil
	}
	return veladClusterOptions{
		raftnode.WithCluster(raftnode.Config{
			Enabled:   true,
			NodeID:    cfg.RaftNodeID,
			BindAddr:  cfg.RaftBind,
			DataDir:   cfg.RaftDataDir,
			Bootstrap: cfg.RaftBoot,
		}),
	}
}

func provideEventBusOptions(bus eventx.BusRuntime) veladEventBusOptions {
	return veladEventBusOptions{vela.WithEventBus(bus)}
}

func provideGatewayOptions(
	base veladBaseOptions,
	metrics veladMetricsOptions,
	configSource veladConfigSourceOptions,
	cluster veladClusterOptions,
	eventBus veladEventBusOptions,
) veladGatewayOptions {
	size := len(base) + len(metrics) + len(configSource) + len(cluster) + len(eventBus)
	options := make([]vela.Option, 0, size)
	options = append(options, base...)
	options = append(options, metrics...)
	options = append(options, configSource...)
	options = append(options, cluster...)
	options = append(options, eventBus...)
	return options
}

func provideGateway(options veladGatewayOptions) (*vela.Gateway, error) {
	gateway, err := vela.New(options...)
	if err != nil {
		return nil, oops.
			In("cmd").
			Wrapf(err, "create gateway")
	}
	return gateway, nil
}

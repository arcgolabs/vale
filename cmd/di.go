package main

import (
	"log/slog"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/arcgolabs/vale"
	raftnode "github.com/arcgolabs/vale/cluster/raftnode"
	prometheusmetrics "github.com/arcgolabs/vale/observability/prometheus"
	fileconfig "github.com/arcgolabs/vale/provider/fileconfig"
	"github.com/samber/oops"
)

const defaultMetricsName = "prometheus"

type (
	valedBaseOptions         []vale.Option
	valedMetricsOptions      []vale.Option
	valedConfigSourceOptions []vale.Option
	valedClusterOptions      []vale.Option
	valedEventBusOptions     []vale.Option
	valedGatewayOptions      []vale.Option
)

func provideLogger(cfg valedConfig) (*slog.Logger, error) {
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

func providePluginRegistry() (*vale.Registry, error) {
	registry := vale.NewRegistry()
	if err := registry.RegisterMetricsFactory(defaultMetricsName, prometheusmetrics.New); err != nil {
		return nil, oops.
			In("cmd").
			With("metrics", defaultMetricsName).
			Wrapf(err, "register metrics factory")
	}
	return registry, nil
}

func provideBaseOptions(cfg valedConfig, logger *slog.Logger) valedBaseOptions {
	return valedBaseOptions{
		vale.WithWatch(cfg.Watch),
		vale.WithLogger(logger),
	}
}

func provideMetricsOptions(registry *vale.Registry) valedMetricsOptions {
	return valedMetricsOptions{
		vale.WithMetricsFromRegistry(registry, defaultMetricsName),
	}
}

func provideConfigSourceOptions(cfg valedConfig) valedConfigSourceOptions {
	files := parseCSV(cfg.ConfigFiles)
	switch {
	case !files.IsEmpty():
		return valedConfigSourceOptions{fileconfig.WithConfigFileList(files)}
	case strings.TrimSpace(cfg.ConfigPath) != "":
		return valedConfigSourceOptions{fileconfig.WithConfigPath(cfg.ConfigPath)}
	default:
		return nil
	}
}

func provideClusterOptions(cfg valedConfig) (valedClusterOptions, error) {
	initialMembers, err := parseRaftInitialMembers(cfg.RaftMembers)
	if err != nil {
		return nil, oops.
			In("cmd").
			With("raft_initial_members", cfg.RaftMembers).
			Wrapf(err, "parse raft initial members")
	}
	raftConfig := raftnode.Config{
		NodeID:    cfg.RaftNodeID,
		BindAddr:  cfg.RaftBind,
		DataDir:   cfg.RaftDataDir,
		Bootstrap: cfg.RaftBoot,
	}
	if !initialMembers.IsEmpty() {
		raftConfig.Groups = collectionlist.NewList(
			raftnode.GroupConfig{
				Name:           raftnode.MetadataGroupName,
				InitialMembers: initialMembers,
			},
			raftnode.GroupConfig{
				Name:           raftnode.DataGroupName,
				InitialMembers: initialMembers,
			},
		)
	}
	return valedClusterOptions{
		vale.WithClusterFactory(func(logger *slog.Logger) (vale.Cluster, error) {
			return raftnode.New(raftConfig, logger)
		}),
	}, nil
}

func provideEventBusOptions(bus eventx.BusRuntime) valedEventBusOptions {
	return valedEventBusOptions{vale.WithEventBus(bus)}
}

func provideGatewayOptions(
	base valedBaseOptions,
	metrics valedMetricsOptions,
	configSource valedConfigSourceOptions,
	cluster valedClusterOptions,
	eventBus valedEventBusOptions,
) valedGatewayOptions {
	size := len(base) + len(metrics) + len(configSource) + len(cluster) + len(eventBus)
	options := make([]vale.Option, 0, size)
	options = append(options, base...)
	options = append(options, metrics...)
	options = append(options, configSource...)
	options = append(options, cluster...)
	options = append(options, eventBus...)
	return options
}

func provideGateway(options valedGatewayOptions) (*vale.Gateway, error) {
	gateway, err := vale.New(options...)
	if err != nil {
		return nil, oops.
			In("cmd").
			Wrapf(err, "create gateway")
	}
	return gateway, nil
}

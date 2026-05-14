package main

import (
	"log/slog"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale"
	raftnode "github.com/arcgolabs/vale/cluster/raftnode"
	prometheusmetrics "github.com/arcgolabs/vale/observability/prometheus"
	fileconfig "github.com/arcgolabs/vale/provider/fileconfig"
	"github.com/samber/oops"
)

const defaultMetricsName = "prometheus"

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
	logger.Debug("logger configured", "level", cfg.LogLevel)
	return logger, nil
}

func provideEventBus() eventx.BusRuntime {
	return eventx.New()
}

func provideObservability(logger *slog.Logger) observabilityx.Observability {
	return prometheusmetrics.NewObservability(logger)
}

func provideExtensionRegistry(obs observabilityx.Observability) (*vale.Registry, error) {
	registry := vale.NewRegistry()
	if err := registry.RegisterMetricsFactory(defaultMetricsName, prometheusmetrics.NewFactory(obs)); err != nil {
		return nil, oops.
			In("cmd").
			With("metrics", defaultMetricsName).
			Wrapf(err, "register metrics factory")
	}
	return registry, nil
}

func provideWatchOption(cfg valedConfig) vale.Option {
	return vale.WithWatch(cfg.Watch)
}

func provideLoggerOption(logger *slog.Logger) vale.Option {
	return vale.WithLogger(logger)
}

func provideObservabilityOption(obs observabilityx.Observability) vale.Option {
	return vale.WithObservability(obs)
}

func provideMetricsOption(registry *vale.Registry) vale.Option {
	return vale.WithMetricsFromRegistry(registry, defaultMetricsName)
}

func provideConfigSourceOption(cfg valedConfig) vale.Option {
	files := parseCSV(cfg.ConfigFiles)
	switch {
	case !files.IsEmpty():
		return fileconfig.WithConfigFileList(files)
	case strings.TrimSpace(cfg.ConfigPath) != "":
		return fileconfig.WithConfigPath(cfg.ConfigPath)
	default:
		return noopGatewayOption
	}
}

func provideClusterOption(cfg valedConfig) (vale.Option, error) {
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
	discoveryConfig, discoveryEnabled, err := clusterDiscoveryConfig(cfg)
	if err != nil {
		return nil, err
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
			raftnode.GroupConfig{
				Name:           raftnode.CertificatesGroupName,
				InitialMembers: initialMembers,
			},
		)
	}
	return vale.WithClusterFactory(func(logger *slog.Logger) (vale.Cluster, error) {
		runtimeConfig := raftConfig
		if discoveryEnabled {
			runtimeConfig.Discovery = raftnode.NewMemberlistDiscovery(discoveryConfig, logger)
		}
		return raftnode.New(runtimeConfig, logger)
	}), nil
}

func clusterDiscoveryConfig(cfg valedConfig) (raftnode.MemberlistDiscoveryConfig, bool, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.ClusterDiscovery))
	switch {
	case mode == "" && strings.TrimSpace(cfg.GossipSeeds) == "":
		return raftnode.MemberlistDiscoveryConfig{}, false, nil
	case mode == "", mode == "gossip":
		return raftnode.MemberlistDiscoveryConfig{
			BindAddr:      cfg.GossipBind,
			AdvertiseAddr: cfg.GossipAdvertise,
			Seeds:         parseCSV(cfg.GossipSeeds),
		}, true, nil
	default:
		return raftnode.MemberlistDiscoveryConfig{}, false, oops.
			In("cmd").
			With("cluster_discovery", cfg.ClusterDiscovery).
			New("unsupported cluster discovery mode")
	}
}

func provideEventBusOption(bus eventx.BusRuntime) vale.Option {
	return vale.WithEventBus(bus)
}

func provideGateway(options *collectionlist.List[vale.Option]) (*vale.Gateway, error) {
	if options == nil {
		options = collectionlist.NewList[vale.Option]()
	}
	gateway, err := vale.New(options.Values()...)
	if err != nil {
		return nil, oops.
			In("cmd").
			Wrapf(err, "create gateway")
	}
	return gateway, nil
}

func noopGatewayOption(*vale.Config) error {
	return nil
}

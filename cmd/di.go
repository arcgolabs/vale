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

func provideWatchComponent(cfg valedConfig) vale.GatewayComponent {
	return vale.GatewayOptions(vale.WithWatch(cfg.Watch))
}

func provideLoggerComponent(logger *slog.Logger) vale.GatewayComponent {
	return vale.GatewayOptions(vale.WithLogger(logger))
}

func provideObservabilityComponent(obs observabilityx.Observability) vale.GatewayComponent {
	return vale.GatewayOptions(vale.WithObservability(obs))
}

func provideMetricsComponent(registry *vale.Registry) vale.GatewayComponent {
	return vale.GatewayOptions(vale.WithMetricsFromRegistry(registry, defaultMetricsName))
}

func provideConfigSourceComponent(cfg valedConfig) vale.GatewayComponent {
	files := parseCSV(cfg.ConfigFiles)
	switch {
	case !files.IsEmpty():
		return vale.GatewayOptions(fileconfig.WithConfigFileList(files))
	case strings.TrimSpace(cfg.ConfigPath) != "":
		return vale.GatewayOptions(fileconfig.WithConfigPath(cfg.ConfigPath))
	default:
		return vale.GatewayComponentFunc(noopGatewayComponent)
	}
}

func provideClusterComponent(cfg valedConfig) (vale.GatewayComponent, error) {
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
	return vale.GatewayOptions(vale.WithClusterFactory(func(logger *slog.Logger) (vale.Cluster, error) {
		runtimeConfig := raftConfig
		if discoveryEnabled {
			runtimeConfig.Discovery = raftnode.NewMemberlistDiscovery(discoveryConfig, logger)
		}
		return raftnode.New(runtimeConfig, logger)
	})), nil
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

func provideEventBusComponent(bus eventx.BusRuntime) vale.GatewayComponent {
	return vale.GatewayOptions(vale.WithEventBus(bus))
}

func provideGateway(components *collectionlist.List[vale.GatewayComponent]) (*vale.Gateway, error) {
	builder := vale.NewGatewayBuilder()
	if components != nil {
		components.Range(func(_ int, component vale.GatewayComponent) bool {
			builder.WithComponents(component)
			return true
		})
	}
	gateway, err := builder.Build()
	if err != nil {
		return nil, oops.
			In("cmd").
			Wrapf(err, "create gateway")
	}
	return gateway, nil
}

func noopGatewayComponent(*vale.GatewayBuilder) error {
	return nil
}

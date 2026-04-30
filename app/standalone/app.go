package standalone

import (
	"context"
	"log/slog"
	"strings"

	"github.com/arcgolabs/configx"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	raftnode "github.com/arcgolabs/gateway/cluster/raftnode"
	gatewayapi "github.com/arcgolabs/gateway/gateway"
	providerevents "github.com/arcgolabs/gateway/provider"
	"github.com/arcgolabs/logx"
)

type Bootstrap struct {
	ConfigPath  string `koanf:"config"`
	ConfigFiles string `koanf:"config_files"`
	Watch       bool   `koanf:"watch"`
	LogLevel    string `koanf:"log_level"`
	RaftEnabled bool   `koanf:"raft_enabled"`
	RaftNodeID  string `koanf:"raft_node_id"`
	RaftBind    string `koanf:"raft_bind"`
	RaftDataDir string `koanf:"raft_data_dir"`
	RaftBoot    bool   `koanf:"raft_bootstrap"`
}

func (b Bootstrap) GatewayOptions(logger *slog.Logger) []gatewayapi.Option {
	options := []gatewayapi.Option{
		gatewayapi.WithConfigPath(b.ConfigPath),
		gatewayapi.WithWatch(b.Watch),
		gatewayapi.WithLogger(logger),
	}
	if b.RaftEnabled {
		options = append(options, gatewayapi.WithRaftCluster(raftnode.Config{
			Enabled:   true,
			NodeID:    b.RaftNodeID,
			BindAddr:  b.RaftBind,
			DataDir:   b.RaftDataDir,
			Bootstrap: b.RaftBoot,
		}))
	}
	files := parseCSV(b.ConfigFiles)
	if len(files) > 0 {
		options = append(options, gatewayapi.WithConfigFiles(files...))
	}
	return options
}

func DefaultBootstrap() Bootstrap {
	return Bootstrap{
		ConfigPath:  "./vela.hcl",
		ConfigFiles: "",
		Watch:       true,
		LogLevel:    "info",
		RaftEnabled: false,
		RaftNodeID:  "node-1",
		RaftBind:    "127.0.0.1:17000",
		RaftDataDir: "./data/raft",
		RaftBoot:    true,
	}
}

func parseCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func LoadBootstrap() (Bootstrap, error) {
	defaults := DefaultBootstrap()
	cfg, err := configx.LoadTErr[Bootstrap](
		configx.WithTypedDefaults(defaults),
		configx.WithEnvPrefix("VELA"),
		configx.WithEnvSeparator("_"),
	)
	if err != nil {
		return defaults, err
	}
	return cfg, nil
}

func NewApp(bootstrap Bootstrap) *dix.App {
	module := dix.NewModule(
		"velad-standalone",
		dix.Providers(
			dix.Value(bootstrap),
			dix.ProviderErr1(func(boot Bootstrap) (*slog.Logger, error) {
				return logx.New(
					logx.WithConsole(true),
					logx.WithLevelString(boot.LogLevel),
				)
			}),
			dix.Provider0(func() eventx.BusRuntime {
				return eventx.New()
			}),
			dix.ProviderErr3(func(boot Bootstrap, logger *slog.Logger, bus eventx.BusRuntime) (*gatewayapi.Gateway, error) {
				options := append(boot.GatewayOptions(logger), gatewayapi.WithEventBus(bus))
				return gatewayapi.New(options...)
			}),
			dix.ProviderErr3(func(logger *slog.Logger, embeddedGateway *gatewayapi.Gateway, bus eventx.BusRuntime) (*Runner, error) {
				return NewRunner(logger, embeddedGateway, bus)
			}),
		),
		dix.Invokes(
			dix.Invoke2(func(bus eventx.BusRuntime, logger *slog.Logger) {
				_, err := eventx.Subscribe[providerevents.ConfigSourceFailedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceFailedEvent) error {
					logger.Error("config source load failed", "source", event.Source, "error", event.Error)
					return nil
				})
				if err != nil {
					logger.Error("failed to subscribe provider events", "error", err)
				}
			}),
		),
	)
	return dix.NewApp("velad", module)
}

type Runner struct {
	logger  *slog.Logger
	gateway *gatewayapi.Gateway
	bus     eventx.BusRuntime
}

func NewRunner(logger *slog.Logger, embeddedGateway *gatewayapi.Gateway, bus eventx.BusRuntime) (*Runner, error) {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runner{
		logger:  logger,
		gateway: embeddedGateway,
		bus:     bus,
	}, nil
}

func (r *Runner) Start(ctx context.Context) error {
	return r.gateway.Start(ctx)
}

func (r *Runner) Stop(ctx context.Context) error {
	_ = r.gateway.Stop(ctx)
	if r.bus != nil {
		_ = r.bus.Close()
	}
	_ = logx.Close(r.logger)
	return nil
}

// Standalone velad process: dix assembly, configx, Cobra, and runtime loop live here;
// main.go only prints errors and sets the exit code.
package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/arcgolabs/configx"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/arcgolabs/vela"
	prometheusmetrics "github.com/arcgolabs/vela/observability/prometheus"
	"github.com/samber/lo"
	"github.com/spf13/pflag"

	raftnode "github.com/arcgolabs/vela/cluster/raftnode"
	providerevents "github.com/arcgolabs/vela/provider"
	fileconfig "github.com/arcgolabs/vela/provider/fileconfig"
)

// veladConfig is the standalone process bootstrap shape (env VELA_*, defaults, changed CLI flags).
type veladConfig struct {
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

func (c veladConfig) gatewayOptions(logger *slog.Logger) []vela.Option {
	options := []vela.Option{
		vela.WithWatch(c.Watch),
		vela.WithLogger(logger),
		prometheusmetrics.WithMetrics(),
	}
	if c.RaftEnabled {
		options = append(options, raftnode.WithCluster(raftnode.Config{
			Enabled:   true,
			NodeID:    c.RaftNodeID,
			BindAddr:  c.RaftBind,
			DataDir:   c.RaftDataDir,
			Bootstrap: c.RaftBoot,
		}))
	}
	files := parseCSV(c.ConfigFiles)
	switch {
	case len(files) > 0:
		options = append(options, fileconfig.WithConfigFiles(files...))
	case strings.TrimSpace(c.ConfigPath) != "":
		options = append(options, fileconfig.WithConfigPath(c.ConfigPath))
	}
	return options
}

// veladStandaloneApp is the sole DI assembly entry for this process.
func veladStandaloneApp(cliFlags *pflag.FlagSet) *dix.App {
	return dix.NewApp("velad", dix.NewModule(
		"velad",
		dix.Providers(
			dix.Value(cliFlags),
			dix.ProviderErr1(provideVeladConfig),
			dix.ProviderErr1(func(cfg veladConfig) (*slog.Logger, error) {
				logger, err := logx.New(
					logx.WithConsole(true),
					logx.WithLevelString(cfg.LogLevel),
					logx.WithGlobalLogger(),
				)
				if err != nil {
					return nil, err
				}
				logx.SetDefault(logger)
				logger.Info("logger configured", "level", cfg.LogLevel)
				return logger, nil
			}),
			dix.Provider0(func() eventx.BusRuntime { return eventx.New() }),
			dix.ProviderErr3(func(cfg veladConfig, logger *slog.Logger, bus eventx.BusRuntime) (*vela.Gateway, error) {
				opts := append(cfg.gatewayOptions(logger), vela.WithEventBus(bus))
				return vela.New(opts...)
			}),
		),
		dix.Invokes(dix.Invoke2(func(bus eventx.BusRuntime, logger *slog.Logger) {
			if _, err := eventx.Subscribe[providerevents.ConfigSourceLoadedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceLoadedEvent) error {
				logger.Info("config source loaded", "source", event.Source, "duration", event.Duration, "size", event.ConfigSize)
				return nil
			}); err != nil {
				logger.Error("failed to subscribe provider event", "event", providerevents.EventNameConfigSourceLoaded, "error", err)
			}
			if _, err := eventx.Subscribe[providerevents.ConfigSourceFailedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceFailedEvent) error {
				logger.Error("config source load failed", "source", event.Source, "error", event.Error)
				return nil
			}); err != nil {
				logger.Error("failed to subscribe provider event", "event", providerevents.EventNameConfigSourceFailed, "error", err)
			}
			if _, err := eventx.Subscribe[providerevents.ConfigSourceChangedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceChangedEvent) error {
				logger.Info("config source changed", "source", event.Source)
				return nil
			}); err != nil {
				logger.Error("failed to subscribe provider event", "event", providerevents.EventNameConfigSourceChanged, "error", err)
			}
			if _, err := eventx.Subscribe[providerevents.SnapshotRecompiledEvent](bus, func(_ context.Context, event providerevents.SnapshotRecompiledEvent) error {
				logger.Info("snapshot recompiled", "sources", event.SourceCount, "routes", event.RouteCount, "services", event.ServiceCount)
				return nil
			}); err != nil {
				logger.Error("failed to subscribe provider event", "event", providerevents.EventNameSnapshotRecompiled, "error", err)
			}
			if _, err := eventx.Subscribe[providerevents.WatchSetupFailedEvent](bus, func(_ context.Context, event providerevents.WatchSetupFailedEvent) error {
				logger.Error("watch setup failed", "source", event.Source, "error", event.Error)
				return nil
			}); err != nil {
				logger.Error("failed to subscribe provider event", "event", providerevents.EventNameWatchSetupFailed, "error", err)
			}
		})),
	))
}

func provideVeladConfig(fs *pflag.FlagSet) (veladConfig, error) {
	def := defaultVeladConfig()
	cfg, err := configx.LoadTErr[veladConfig](
		configx.WithTypedDefaults(def),
		configx.WithEnvPrefix("VELA"),
		configx.WithEnvSeparator("_"),
		configx.WithFlagSet(fs),
		configx.WithArgsNameFunc(cliFlagKoanfPath),
	)
	if err != nil {
		return def, err
	}
	return cfg, nil
}

func defaultVeladConfig() veladConfig {
	return veladConfig{
		ConfigPath:  "",
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

func cliFlagKoanfPath(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "-", "_")
}

func parseCSV(input string) []string {
	if strings.TrimSpace(input) == "" {
		return nil
	}
	return lo.FilterMap(strings.Split(input, ","), func(part string, _ int) (string, bool) {
		trimmed := strings.TrimSpace(part)
		return trimmed, trimmed != ""
	})
}

func execute() error {
	return rootCmd.Execute()
}

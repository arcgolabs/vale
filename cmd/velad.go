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
	"github.com/spf13/pflag"

	raftnode "github.com/arcgolabs/vela/cluster/raftnode"
	providerevents "github.com/arcgolabs/vela/provider"
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
	}
	if c.RaftEnabled {
		options = append(options, vela.WithRaftCluster(raftnode.Config{
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
		options = append(options, vela.WithConfigFiles(files...))
	case strings.TrimSpace(c.ConfigPath) != "":
		options = append(options, vela.WithConfigPath(c.ConfigPath))
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
				return logx.New(
					logx.WithConsole(true),
					logx.WithLevelString(cfg.LogLevel),
				)
			}),
			dix.Provider0(func() eventx.BusRuntime { return eventx.New() }),
			dix.ProviderErr3(func(cfg veladConfig, logger *slog.Logger, bus eventx.BusRuntime) (*vela.Gateway, error) {
				opts := append(cfg.gatewayOptions(logger), vela.WithEventBus(bus))
				return vela.New(opts...)
			}),
		),
		dix.Invokes(dix.Invoke2(func(bus eventx.BusRuntime, logger *slog.Logger) {
			if _, err := eventx.Subscribe[providerevents.ConfigSourceFailedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceFailedEvent) error {
				logger.Error("config source load failed", "source", event.Source, "error", event.Error)
				return nil
			}); err != nil {
				logger.Error("failed to subscribe provider events", "error", err)
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

func execute() error {
	return rootCmd.Execute()
}

// Standalone velad process: dix assembly, configx, Cobra, and runtime loop live here;
// main.go only prints errors and sets the exit code.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/arcgolabs/configx"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	raftnode "github.com/arcgolabs/gateway/cluster/raftnode"
	gw "github.com/arcgolabs/gateway/gateway"
	providerevents "github.com/arcgolabs/gateway/provider"
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

func (c veladConfig) gatewayOptions(logger *slog.Logger) []gw.Option {
	options := []gw.Option{
		gw.WithConfigPath(c.ConfigPath),
		gw.WithWatch(c.Watch),
		gw.WithLogger(logger),
	}
	if c.RaftEnabled {
		options = append(options, gw.WithRaftCluster(raftnode.Config{
			Enabled:   true,
			NodeID:    c.RaftNodeID,
			BindAddr:  c.RaftBind,
			DataDir:   c.RaftDataDir,
			Bootstrap: c.RaftBoot,
		}))
	}
	files := parseCSV(c.ConfigFiles)
	if len(files) > 0 {
		options = append(options, gw.WithConfigFiles(files...))
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
			dix.ProviderErr3(func(cfg veladConfig, logger *slog.Logger, bus eventx.BusRuntime) (*gw.Gateway, error) {
				opts := append(cfg.gatewayOptions(logger), gw.WithEventBus(bus))
				return gw.New(opts...)
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

var rootCmd = &cobra.Command{
	Use:           "velad",
	Short:         "Vela application gateway daemon",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          runVelad,
}

func init() {
	f := rootCmd.Flags()
	f.String("config", "", "path to vela HCL config")
	f.String("config-files", "", "comma-separated config files (merge order: left to right)")
	f.Bool("watch", false, "watch config and hot reload")
	f.String("log-level", "", "log level")
	f.Bool("raft-enabled", false, "enable raft control-plane node")
	f.String("raft-node-id", "", "raft node id")
	f.String("raft-bind", "", "raft bind address")
	f.String("raft-data-dir", "", "raft data directory")
	f.Bool("raft-bootstrap", false, "bootstrap raft cluster if no existing state")
}

func runVelad(cmd *cobra.Command, _ []string) error {
	rt, err := veladStandaloneApp(cmd.Flags()).Build()
	if err != nil {
		return fmt.Errorf("dix build: %w", err)
	}
	c := rt.Container()

	gateway, err := dix.ResolveAs[*gw.Gateway](c)
	if err != nil {
		return fmt.Errorf("resolve gateway: %w", err)
	}
	logger, err := dix.ResolveAs[*slog.Logger](c)
	if err != nil {
		return fmt.Errorf("resolve logger: %w", err)
	}
	bus, err := dix.ResolveAs[eventx.BusRuntime](c)
	if err != nil {
		return fmt.Errorf("resolve event bus: %w", err)
	}

	if err := gateway.Start(context.Background()); err != nil {
		return fmt.Errorf("start velad: %w", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = gateway.Stop(ctx)
	if bus != nil {
		_ = bus.Close()
	}
	_ = logx.Close(logger)
	return nil
}

func execute() error {
	return rootCmd.Execute()
}

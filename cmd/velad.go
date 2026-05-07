// Standalone velad process: dix assembly, configx, Cobra, and runtime loop live here;
// main.go only prints errors and sets the exit code.
package main

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/configx"
	"github.com/arcgolabs/dix"
	"github.com/samber/oops"
	"github.com/spf13/pflag"
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

// veladStandaloneApp is the sole DI assembly entry for this process.
func veladStandaloneApp(cliFlags *pflag.FlagSet) *dix.App {
	return dix.NewApp("velad", dix.NewModule(
		"velad",
		dix.Providers(
			dix.Value(cliFlags),
			dix.ProviderErr1(provideVeladConfig),
			dix.ProviderErr1(provideLogger),
			dix.Provider0(provideEventBus),
			dix.ProviderErr0(providePluginRegistry),
			dix.Provider2(provideBaseOptions),
			dix.Provider1(provideMetricsOptions),
			dix.Provider1(provideConfigSourceOptions),
			dix.Provider1(provideClusterOptions),
			dix.Provider1(provideEventBusOptions),
			dix.Provider5(provideGatewayOptions),
			dix.ProviderErr1(provideGateway),
			dix.Provider3(provideRunner),
		),
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
	return collectionlist.FilterMapList(collectionlist.NewList(strings.Split(input, ",")...), func(_ int, part string) (string, bool) {
		trimmed := strings.TrimSpace(part)
		return trimmed, trimmed != ""
	}).Values()
}

func execute() error {
	if err := rootCmd.Execute(); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "execute root command")
	}
	return nil
}

// Standalone valed process: dix assembly, configx, Cobra, and runtime loop live here;
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

// valedConfig is the standalone process bootstrap shape (env VALE_*, defaults, changed CLI flags).
type valedConfig struct {
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

// valedStandaloneApp is the sole DI assembly entry for this process.
func valedStandaloneApp(cliFlags *pflag.FlagSet) *dix.App {
	return dix.NewApp("valed", dix.NewModule(
		"valed",
		dix.Providers(
			dix.Value(cliFlags),
			dix.ProviderErr1(provideValedConfig),
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

func provideValedConfig(fs *pflag.FlagSet) (valedConfig, error) {
	def := defaultValedConfig()
	cfg, err := configx.LoadTErr[valedConfig](
		configx.WithTypedDefaults(def),
		configx.WithEnvPrefix("VALE"),
		configx.WithEnvSeparator("_"),
		configx.WithFlagSet(fs),
		configx.WithArgsNameFunc(cliFlagKoanfPath),
	)
	if err != nil {
		return def, err
	}
	return cfg, nil
}

func defaultValedConfig() valedConfig {
	return valedConfig{
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

func parseCSV(input string) *collectionlist.List[string] {
	if strings.TrimSpace(input) == "" {
		return collectionlist.NewList[string]()
	}
	return collectionlist.FilterMapList(collectionlist.NewList(strings.Split(input, ",")...), func(_ int, part string) (string, bool) {
		trimmed := strings.TrimSpace(part)
		return trimmed, trimmed != ""
	})
}

func execute() error {
	if err := rootCmd.Execute(); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "execute root command")
	}
	return nil
}

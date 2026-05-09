// Standalone valed process: dix assembly, configx, Cobra, and runtime loop live here;
// main.go only prints errors and sets the exit code.
package main

import (
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/configx"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/vale"
	"github.com/samber/oops"
	"github.com/spf13/pflag"
)

// valedConfig is the standalone process bootstrap shape (env VALE_*, defaults, changed CLI flags).
type valedConfig struct {
	ConfigPath  string `koanf:"config"`
	ConfigFiles string `koanf:"config_files"`
	Watch       bool   `koanf:"watch"`
	LogLevel    string `koanf:"log_level"`
	RaftNodeID  string `koanf:"raft_node_id"`
	RaftBind    string `koanf:"raft_bind"`
	RaftDataDir string `koanf:"raft_data_dir"`
	RaftBoot    bool   `koanf:"raft_bootstrap"`
	RaftMembers string `koanf:"raft_initial_members"`
}

// valedStandaloneApp is the sole DI assembly entry for this process.
func valedStandaloneApp(cliFlags *pflag.FlagSet) *dix.App {
	return dix.New(
		"valed",
		dix.RecentEvents(256),
		dix.Modules(dix.NewModule(
			"valed",
			dix.Providers(
				dix.Value(cliFlags),
				dix.ProviderErr1(provideValedConfig, dix.Eager()),
				dix.ProviderErr1(provideLogger, dix.Eager()),
				dix.Provider0(provideEventBus, dix.Eager()),
				dix.Provider1(provideObservability, dix.Eager()),
				dix.Provider1(provideDixObserver),
				dix.ProviderErr1(providePluginRegistry, dix.Eager()),
				dix.Contribute1[vale.Option](provideWatchOption, dix.Order(0)),
				dix.Contribute1[vale.Option](provideLoggerOption, dix.Order(10)),
				dix.Contribute1[vale.Option](provideObservabilityOption, dix.Order(20)),
				dix.Contribute1[vale.Option](provideMetricsOption, dix.Order(30)),
				dix.Contribute1[vale.Option](provideConfigSourceOption, dix.Order(40)),
				dix.ContributeErr1[vale.Option](provideClusterOption, dix.Order(50)),
				dix.Contribute1[vale.Option](provideEventBusOption, dix.Order(60)),
				dix.ProviderErr1(provideGateway, dix.Eager()),
				dix.Provider3(provideRunner),
			),
		)),
	)
}

func provideValedConfig(fs *pflag.FlagSet) (valedConfig, error) {
	def := defaultValedConfig()
	cfg, err := configx.LoadTErr[valedConfig](
		configx.WithDefaults(defaultValedConfigValues(def)),
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
		RaftNodeID:  "node-1",
		RaftBind:    "127.0.0.1:17000",
		RaftDataDir: "./data/raft",
		RaftBoot:    true,
		RaftMembers: "",
	}
}

func defaultValedConfigValues(cfg valedConfig) map[string]any {
	return map[string]any{
		"config":               cfg.ConfigPath,
		"config_files":         cfg.ConfigFiles,
		"watch":                cfg.Watch,
		"log_level":            cfg.LogLevel,
		"raft_node_id":         cfg.RaftNodeID,
		"raft_bind":            cfg.RaftBind,
		"raft_data_dir":        cfg.RaftDataDir,
		"raft_bootstrap":       cfg.RaftBoot,
		"raft_initial_members": cfg.RaftMembers,
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

func parseRaftInitialMembers(input string) (*mapping.Map[string, string], error) {
	members := mapping.NewMap[string, string]()
	var parseErr error
	parseCSV(input).Range(func(_ int, part string) bool {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 {
			parseErr = fmt.Errorf("raft initial member %q must use id=address form", part)
			return false
		}
		id := strings.TrimSpace(pair[0])
		address := strings.TrimSpace(pair[1])
		if id == "" || address == "" {
			parseErr = fmt.Errorf("raft initial member %q has empty id or address", part)
			return false
		}
		if _, exists := members.Get(id); exists {
			parseErr = fmt.Errorf("raft initial member %q is duplicated", id)
			return false
		}
		members.Set(id, address)
		return true
	})
	if parseErr != nil {
		return nil, parseErr
	}
	return members, nil
}

func execute() error {
	if err := rootCmd.Execute(); err != nil {
		return oops.
			In("cmd").
			Wrapf(err, "execute root command")
	}
	return nil
}

package main

import (
	"log/slog"
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/vale"
	"github.com/spf13/pflag"
)

func TestValedDefaultOptionsCreateGatewayWithoutConfigFile(t *testing.T) {
	t.Parallel()

	cfg := defaultValedConfig()
	registry, err := providePluginRegistry()
	if err != nil {
		t.Fatal(err)
	}
	clusterOptions, err := provideClusterOptions(cfg)
	if err != nil {
		t.Fatal(err)
	}
	options := provideGatewayOptions(
		provideBaseOptions(cfg, slog.New(slog.DiscardHandler)),
		provideMetricsOptions(registry),
		provideConfigSourceOptions(cfg),
		clusterOptions,
		nil,
	)
	gateway, err := vale.New(options...)
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
	}
}

func TestValedStandaloneAppResolvesRunner(t *testing.T) {
	t.Parallel()

	rt, err := valedStandaloneApp(pflag.NewFlagSet("valed-test", pflag.ContinueOnError)).Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dix.ResolveAs[*valedRunner](rt.Container()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultValedConfigDoesNotRequireConfigPath(t *testing.T) {
	t.Parallel()

	cfg := defaultValedConfig()
	if cfg.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty", cfg.ConfigPath)
	}
}

func TestParseRaftInitialMembers(t *testing.T) {
	t.Parallel()

	members, err := parseRaftInitialMembers("node-1=vale-1:17000,node-2=vale-2:17000")
	if err != nil {
		t.Fatal(err)
	}
	if members.Len() != 2 {
		t.Fatalf("members.Len() = %d, want 2", members.Len())
	}
	address, ok := members.Get("node-2")
	if !ok || address != "vale-2:17000" {
		t.Fatalf("node-2 address = %q, %v; want vale-2:17000, true", address, ok)
	}
}

func TestParseRaftInitialMembersRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	if _, err := parseRaftInitialMembers("node-1=vale-1:17000,node-1=vale-2:17000"); err == nil {
		t.Fatal("expected duplicate member error")
	}
}

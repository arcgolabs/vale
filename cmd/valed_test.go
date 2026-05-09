package main

import (
	"log/slog"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/dix/testx"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/spf13/pflag"
)

func TestValedDefaultOptionsCreateGatewayWithoutConfigFile(t *testing.T) {
	t.Parallel()

	cfg := defaultValedConfig()
	obs := observabilityx.Nop()
	registry, err := providePluginRegistry(obs)
	if err != nil {
		t.Fatal(err)
	}
	clusterOption, err := provideClusterOption(cfg)
	if err != nil {
		t.Fatal(err)
	}
	bus := eventx.New()
	t.Cleanup(func() {
		if closeErr := bus.Close(); closeErr != nil {
			t.Fatalf("close bus: %v", closeErr)
		}
	})
	options := collectionlist.NewList(
		provideWatchOption(cfg),
		provideLoggerOption(slog.New(slog.DiscardHandler)),
		provideObservabilityOption(obs),
		provideMetricsOption(registry),
		provideConfigSourceOption(cfg),
		clusterOption,
		provideEventBusOption(bus),
	)
	gateway, err := provideGateway(options)
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
	}
}

func TestValedStandaloneAppResolvesRunner(t *testing.T) {
	t.Parallel()

	rt := testx.Build(t, valedStandaloneApp(pflag.NewFlagSet("valed-test", pflag.ContinueOnError)))
	if _, err := dix.ResolveAs[*valedRunner](rt.Container()); err != nil {
		t.Fatal(err)
	}
	if rt.EventRecorder() == nil {
		t.Fatal("expected dix recent event recorder")
	}
}

func TestValedStandaloneAppValidatesDependencyGraph(t *testing.T) {
	t.Parallel()

	testx.Validate(t, valedStandaloneApp(pflag.NewFlagSet("valed-test", pflag.ContinueOnError)))
}

func TestDefaultValedConfigDoesNotRequireConfigPath(t *testing.T) {
	t.Parallel()

	cfg := defaultValedConfig()
	if cfg.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty", cfg.ConfigPath)
	}
}

func TestProvideValedConfigKeepsDefaultsForUnchangedFlags(t *testing.T) {
	t.Parallel()

	cfg, err := provideValedConfig(newValedTestFlagSet(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RaftNodeID != "node-1" {
		t.Fatalf("RaftNodeID = %q, want node-1", cfg.RaftNodeID)
	}
	if cfg.RaftBind != "127.0.0.1:17000" {
		t.Fatalf("RaftBind = %q, want 127.0.0.1:17000", cfg.RaftBind)
	}
	if cfg.RaftDataDir != "./data/raft" {
		t.Fatalf("RaftDataDir = %q, want ./data/raft", cfg.RaftDataDir)
	}
	if !cfg.RaftBoot {
		t.Fatal("RaftBoot = false, want true")
	}
}

func TestProvideValedConfigAppliesChangedFlags(t *testing.T) {
	t.Parallel()

	fs := newValedTestFlagSet(t)
	if err := fs.Parse([]string{
		"--raft-node-id", "node-9",
		"--raft-data-dir", "/tmp/vale/raft",
		"--raft-bootstrap=false",
	}); err != nil {
		t.Fatal(err)
	}
	cfg, err := provideValedConfig(fs)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RaftNodeID != "node-9" {
		t.Fatalf("RaftNodeID = %q, want node-9", cfg.RaftNodeID)
	}
	if cfg.RaftDataDir != "/tmp/vale/raft" {
		t.Fatalf("RaftDataDir = %q, want /tmp/vale/raft", cfg.RaftDataDir)
	}
	if cfg.RaftBoot {
		t.Fatal("RaftBoot = true, want false")
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

func newValedTestFlagSet(t *testing.T) *pflag.FlagSet {
	t.Helper()

	fs := pflag.NewFlagSet("valed-test", pflag.ContinueOnError)
	fs.String("config", "", "")
	fs.String("config-files", "", "")
	fs.Bool("watch", false, "")
	fs.String("log-level", "", "")
	fs.String("raft-node-id", "", "")
	fs.String("raft-bind", "", "")
	fs.String("raft-data-dir", "", "")
	fs.Bool("raft-bootstrap", false, "")
	fs.String("raft-initial-members", "", "")
	return fs
}

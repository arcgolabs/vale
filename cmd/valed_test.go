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
	options := provideGatewayOptions(
		provideBaseOptions(cfg, slog.New(slog.DiscardHandler)),
		provideMetricsOptions(registry),
		provideConfigSourceOptions(cfg),
		provideClusterOptions(cfg),
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

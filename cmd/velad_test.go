package main

import (
	"log/slog"
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/vale"
	"github.com/spf13/pflag"
)

func TestVeladDefaultOptionsCreateGatewayWithoutConfigFile(t *testing.T) {
	t.Parallel()

	cfg := defaultVeladConfig()
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
	gateway, err := vela.New(options...)
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
	}
}

func TestVeladStandaloneAppResolvesRunner(t *testing.T) {
	t.Parallel()

	rt, err := veladStandaloneApp(pflag.NewFlagSet("velad-test", pflag.ContinueOnError)).Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dix.ResolveAs[*veladRunner](rt.Container()); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultVeladConfigDoesNotRequireConfigPath(t *testing.T) {
	t.Parallel()

	cfg := defaultVeladConfig()
	if cfg.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty", cfg.ConfigPath)
	}
}

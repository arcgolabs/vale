package main

import (
	"log/slog"
	"testing"

	"github.com/arcgolabs/vela"
)

func TestVeladDefaultOptionsCreateGatewayWithoutConfigFile(t *testing.T) {
	t.Parallel()

	cfg := defaultVeladConfig()
	gateway, err := vela.New(cfg.gatewayOptions(slog.New(slog.DiscardHandler))...)
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
	}
}

func TestDefaultVeladConfigDoesNotRequireConfigPath(t *testing.T) {
	t.Parallel()

	cfg := defaultVeladConfig()
	if cfg.ConfigPath != "" {
		t.Fatalf("ConfigPath = %q, want empty", cfg.ConfigPath)
	}
}

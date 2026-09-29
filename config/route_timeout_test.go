package config_test

import (
	"strings"
	"testing"

	"github.com/arcgolabs/vale/config"
)

func TestValidateRouteWriteTimeout(t *testing.T) {
	t.Parallel()

	for _, timeout := range []string{"", "0s", "250ms"} {
		t.Run(timeout, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Routes[0].WriteTimeout = timeout
			if err := config.Validate(cfg); err != nil {
				t.Fatalf("Validate(write_timeout=%q) error = %v", timeout, err)
			}
		})
	}
}

func TestValidateRejectsInvalidRouteWriteTimeout(t *testing.T) {
	t.Parallel()

	for _, timeout := range []string{"eventually", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Routes[0].WriteTimeout = timeout
			err := config.Validate(cfg)
			if err == nil || !strings.Contains(err.Error(), "write_timeout") {
				t.Fatalf("Validate(write_timeout=%q) error = %v, want write_timeout validation error", timeout, err)
			}
		})
	}
}

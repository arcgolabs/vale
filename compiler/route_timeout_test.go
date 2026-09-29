package compiler_test

import (
	"strings"
	"testing"
	"time"

	"github.com/arcgolabs/vale/compiler"
	"github.com/arcgolabs/vale/config"
)

func TestCompileRouteWriteTimeoutPreservesThreeStates(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		raw     string
		wantNil bool
		want    time.Duration
	}{
		{name: "inherit", wantNil: true},
		{name: "disabled", raw: "0s"},
		{name: "overridden", raw: "250ms", want: 250 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertCompiledRouteWriteTimeout(t, tt.raw, tt.wantNil, tt.want)
		})
	}
}

func TestCompileRejectsInvalidRouteWriteTimeout(t *testing.T) {
	t.Parallel()

	for _, timeout := range []string{"eventually", "-1s"} {
		t.Run(timeout, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Routes[0].WriteTimeout = timeout
			_, err := compiler.Compile(cfg)
			if err == nil || !strings.Contains(err.Error(), "write_timeout") {
				t.Fatalf("Compile(write_timeout=%q) error = %v, want validation error", timeout, err)
			}
		})
	}
}

func assertCompiledRouteWriteTimeout(t *testing.T, raw string, wantNil bool, want time.Duration) {
	t.Helper()
	cfg := config.Default()
	cfg.Routes[0].WriteTimeout = raw
	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	routes := snapshot.RoutesByEntrypoint.Get("web")
	if len(routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(routes))
	}
	got := routes[0].WriteTimeout
	if wantNil {
		if got != nil {
			t.Fatalf("write timeout = %v, want inherited nil", *got)
		}
		return
	}
	if got == nil || *got != want {
		t.Fatalf("write timeout = %v, want %v", got, want)
	}
}

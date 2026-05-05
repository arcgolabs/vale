package gateway

import (
	"context"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/arcgolabs/gateway/runtime"
)

func TestStartReturnsEntrypointListenError(t *testing.T) {
	t.Parallel()

	occupied := listenOnLocalhost(t)
	defer occupied.Close()

	g, err := New(
		WithStaticSnapshot(&runtime.CompiledSnapshot{
			Entrypoints:  map[string]string{"web": occupied.Addr().String()},
			AdminAddress: "127.0.0.1:0",
		}),
		WithWatch(false),
		WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = g.Start(context.Background())
	if err == nil {
		t.Fatal("Start returned nil error, want listen error")
	}
	if !strings.Contains(err.Error(), "listen entrypoint") {
		t.Fatalf("Start error = %q, want entrypoint listen error", err.Error())
	}
}

func TestStartReturnsAdminListenError(t *testing.T) {
	t.Parallel()

	occupied := listenOnLocalhost(t)
	defer occupied.Close()

	g, err := New(
		WithStaticSnapshot(&runtime.CompiledSnapshot{
			Entrypoints:  map[string]string{"web": "127.0.0.1:0"},
			AdminAddress: occupied.Addr().String(),
		}),
		WithWatch(false),
		WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = g.Start(context.Background())
	if err == nil {
		t.Fatal("Start returned nil error, want listen error")
	}
	if !strings.Contains(err.Error(), "listen admin") {
		t.Fatalf("Start error = %q, want admin listen error", err.Error())
	}
}

func TestStaticRuntimeChanges(t *testing.T) {
	t.Parallel()

	current := &runtime.CompiledSnapshot{
		Entrypoints:      map[string]string{"web": "127.0.0.1:8080"},
		AdminAddress:     "127.0.0.1:19090",
		AccessLogEnabled: true,
		MetricsEnabled:   true,
		HealthInterval:   "5s",
		HealthTimeout:    "2s",
	}
	next := &runtime.CompiledSnapshot{
		Entrypoints:      map[string]string{"web": "127.0.0.1:8081"},
		AdminAddress:     "127.0.0.1:19091",
		AccessLogEnabled: false,
		MetricsEnabled:   false,
		HealthInterval:   "10s",
		HealthTimeout:    "3s",
	}

	got := staticRuntimeChanges(current, next)
	want := []string{
		"access_log_enabled",
		"admin_address",
		"entrypoints",
		"health_interval",
		"health_timeout",
		"metrics_enabled",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestStaticRuntimeChangesIgnoresDynamicSnapshotFields(t *testing.T) {
	t.Parallel()

	current := &runtime.CompiledSnapshot{
		Entrypoints:      map[string]string{"web": "127.0.0.1:8080"},
		AdminAddress:     "127.0.0.1:19090",
		AccessLogEnabled: true,
		MetricsEnabled:   true,
		HealthInterval:   "5s",
		HealthTimeout:    "2s",
		ProxyEngine:      "stdlib",
	}
	next := &runtime.CompiledSnapshot{
		Entrypoints:      map[string]string{"web": "127.0.0.1:8080"},
		AdminAddress:     "127.0.0.1:19090",
		AccessLogEnabled: true,
		MetricsEnabled:   true,
		HealthInterval:   "5s",
		HealthTimeout:    "2s",
		ProxyEngine:      "oxy",
	}

	got := staticRuntimeChanges(current, next)
	if len(got) != 0 {
		t.Fatalf("changes = %v, want none", got)
	}
}

func listenOnLocalhost(t *testing.T) net.Listener {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

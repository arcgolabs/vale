package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/compiler"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/runtime"
)

func TestStartReturnsEntrypointListenError(t *testing.T) {
	t.Parallel()

	occupied := listenOnLocalhost(t)
	defer occupied.Close()

	g, err := New(
		WithStaticSnapshot(&runtime.CompiledSnapshot{
			Entrypoints:  mapping.NewMapFrom(map[string]string{"web": occupied.Addr().String()}),
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
			Entrypoints:  mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:0"}),
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
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8080"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19090",
		AccessLogEnabled:  true,
		MetricsEnabled:    true,
		HealthInterval:    "5s",
		HealthTimeout:     "2s",
	}
	next := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8081"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19091",
		AccessLogEnabled:  false,
		MetricsEnabled:    false,
		HealthInterval:    "10s",
		HealthTimeout:     "3s",
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
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestStaticRuntimeChangesIgnoresDynamicSnapshotFields(t *testing.T) {
	t.Parallel()

	current := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8080"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19090",
		AccessLogEnabled:  true,
		MetricsEnabled:    true,
		HealthInterval:    "5s",
		HealthTimeout:     "2s",
		ProxyEngine:       "",
	}
	next := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8080"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19090",
		AccessLogEnabled:  true,
		MetricsEnabled:    true,
		HealthInterval:    "5s",
		HealthTimeout:     "2s",
		ProxyEngine:       "oxy",
	}

	got := staticRuntimeChanges(current, next)
	if !got.IsEmpty() {
		t.Fatalf("changes = %v, want none", got)
	}
}

func TestAdminAPIWritesPlainJSONViews(t *testing.T) {
	t.Parallel()

	snapshot, err := compiler.Compile(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	gateway := &Gateway{
		runtime: runtime.NewGateway(snapshot, discardLogger(), false, runtime.NewNoopMetrics()),
		logger:  discardLogger(),
	}
	mux := gateway.buildAdminMux()

	routeRecorder := httptest.NewRecorder()
	mux.ServeHTTP(routeRecorder, httptest.NewRequest(http.MethodGet, "/admin/routes", nil))
	var routes []runtime.RouteView
	if err := json.Unmarshal(routeRecorder.Body.Bytes(), &routes); err != nil {
		t.Fatalf("routes json decode failed: %v; body=%s", err, routeRecorder.Body.String())
	}
	if len(routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(routes))
	}

	serviceRecorder := httptest.NewRecorder()
	mux.ServeHTTP(serviceRecorder, httptest.NewRequest(http.MethodGet, "/admin/services", nil))
	var services []adminServiceView
	if err := json.Unmarshal(serviceRecorder.Body.Bytes(), &services); err != nil {
		t.Fatalf("services json decode failed: %v; body=%s", err, serviceRecorder.Body.String())
	}
	if len(services) != 1 || len(services[0].Endpoints) != 1 {
		t.Fatalf("services = %#v, want one service with one endpoint", services)
	}

	endpointRecorder := httptest.NewRecorder()
	mux.ServeHTTP(endpointRecorder, httptest.NewRequest(http.MethodGet, "/admin/endpoints", nil))
	var endpoints []runtime.EndpointView
	if err := json.Unmarshal(endpointRecorder.Body.Bytes(), &endpoints); err != nil {
		t.Fatalf("endpoints json decode failed: %v; body=%s", err, endpointRecorder.Body.String())
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints len = %d, want 1", len(endpoints))
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

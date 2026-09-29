package runtime_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestHealthCheckerSkipsHandlerEndpointWhenHTTPUpstreamIsUnhealthy(t *testing.T) {
	modelUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "model unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(modelUpstream.Close)

	modelEndpoint, err := valeruntime.NewEndpoint(modelUpstream.URL, 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	staticEndpoint, err := valeruntime.NewHandlerEndpoint("static", 1, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := io.WriteString(w, "static-ready"); writeErr != nil {
			t.Errorf("write static response: %v", writeErr)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}

	modelService := valeruntime.NewService("model", "round_robin", modelEndpoint)
	staticService := valeruntime.NewService("static", "round_robin", staticEndpoint)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":0", valeruntime.EntrypointRuntime{Name: "web", Address: ":0"}).
		AddService(modelService).
		AddService(staticService).
		AddRoute(valeruntime.NewRoute("model", "web", modelService).WithPathPrefix("/model")).
		AddRoute(valeruntime.NewRoute("static", "web", staticService).WithPathPrefix("/static")).
		BuildMatchers()
	gateway := valeruntime.NewGateway(snapshot, nil, false, valeruntime.NewNoopMetrics())
	checker := valeruntime.NewHealthChecker(time.Millisecond, time.Second)
	checker.Start(t.Context(), gateway)
	defer checker.Stop()

	waitForHealthState(t, modelEndpoint, false)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/static", http.NoBody)
	response := httptest.NewRecorder()
	gateway.Handler("web").ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("handler endpoint status = %d, want %d", response.Code, http.StatusOK)
	}
	if body := strings.TrimSpace(response.Body.String()); body != "static-ready" {
		t.Fatalf("handler endpoint body = %q, want static-ready", body)
	}
	if !staticEndpoint.Healthy.Load() {
		t.Fatal("handler endpoint became unhealthy after HTTP health check cycle")
	}
	if lastChecked := staticEndpoint.LastChecked.Load(); lastChecked != 0 {
		t.Fatalf("handler endpoint last checked = %d, want 0", lastChecked)
	}
}

func TestHealthCheckerRunsEndpointChecksConcurrently(t *testing.T) {
	var active atomic.Int64
	var maxActive atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		updateMaxActive(&maxActive, current)
		defer active.Add(-1)

		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	const endpointCount = 8
	endpoints := collectionlist.NewListWithCapacity[*valeruntime.EndpointRuntime](endpointCount)
	for range endpointCount {
		endpoint, err := valeruntime.NewEndpoint(server.URL, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		endpoints.Add(endpoint)
	}

	service := &valeruntime.ServiceRuntime{
		Name:      "api",
		Strategy:  "round_robin",
		Endpoints: endpoints,
	}
	service.BuildSlots()
	gateway := valeruntime.NewGateway(valeruntime.NewSnapshot().AddService(service), nil, false, valeruntime.NewNoopMetrics())
	if gateway.Snapshot().Services.Len() != 1 {
		t.Fatalf("snapshot services = %d, want 1", gateway.Snapshot().Services.Len())
	}
	if service.Endpoints.Len() != endpointCount {
		t.Fatalf("service endpoints = %d, want %d", service.Endpoints.Len(), endpointCount)
	}

	checker := valeruntime.NewHealthChecker(time.Millisecond, time.Second)
	checker.Start(t.Context(), gateway)
	defer checker.Stop()

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for maxActive.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("max concurrent health checks = %d, want at least 2", maxActive.Load())
		case <-ticker.C:
		}
	}
}

func updateMaxActive(maxActive *atomic.Int64, current int64) {
	for {
		previous := maxActive.Load()
		if current <= previous || maxActive.CompareAndSwap(previous, current) {
			return
		}
	}
}

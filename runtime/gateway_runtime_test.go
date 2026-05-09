package runtime_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestGatewayLogsFailedRequestWhenAccessLogEnabled(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelError}))
	endpoint, err := valeruntime.NewEndpoint(
		"http://127.0.0.1:8081",
		1,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "bad gateway", http.StatusBadGateway)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	service := valeruntime.NewService("api", "round_robin", endpoint)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":0", valeruntime.EntrypointRuntime{Name: "web", Address: ":0"}).
		AddService(service).
		AddRoute(valeruntime.NewRoute("api", "web", service).WithPathPrefix("/")).
		BuildMatchers()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com/", http.NoBody)
	recorder := httptest.NewRecorder()

	valeruntime.NewGateway(snapshot, logger, true, valeruntime.NewNoopMetrics()).
		Handler("web").
		ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadGateway)
	}
	got := logs.String()
	if !strings.Contains(got, "request failed") || !strings.Contains(got, "status_code=502") {
		t.Fatalf("logs = %q, want failed request with status code", got)
	}
}

func TestGatewayPrebuildsRouteEndpointMiddlewareHandler(t *testing.T) {
	t.Parallel()

	var builds atomic.Int64
	var calls atomic.Int64
	registry := valeruntime.NewMiddlewareRegistry()
	if err := registry.Register("count", func(next http.Handler, _ valeruntime.MiddlewareRuntime) http.Handler {
		builds.Add(1)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			next.ServeHTTP(w, r)
		})
	}); err != nil {
		t.Fatal(err)
	}

	endpoint, err := valeruntime.NewEndpoint(
		"http://127.0.0.1:8081",
		1,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	service := valeruntime.NewService("api", "round_robin", endpoint)
	route := valeruntime.NewRoute("api", "web", service).
		WithPathPrefix("/").
		WithMiddleware(valeruntime.MiddlewareRuntime{Name: "count", Type: "count"})
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":0", valeruntime.EntrypointRuntime{Name: "web", Address: ":0"}).
		AddService(service).
		AddRoute(route).
		BuildMatchers()

	handler := valeruntime.NewGatewayWithMiddlewareRegistry(snapshot, nil, false, valeruntime.NewNoopMetrics(), registry).Handler("web")
	for range 2 {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com/", http.NoBody)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
		}
	}

	if builds.Load() != 1 {
		t.Fatalf("middleware handler builds = %d, want 1", builds.Load())
	}
	if calls.Load() != 2 {
		t.Fatalf("middleware calls = %d, want 2", calls.Load())
	}
}

func TestGatewayRouteMatchCacheSkipsHeaderPredicates(t *testing.T) {
	t.Parallel()

	tenantA, err := valeruntime.NewEndpoint(
		"http://127.0.0.1:8081",
		1,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, err := w.Write([]byte("tenant-a")); err != nil {
				return
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	tenantB, err := valeruntime.NewEndpoint(
		"http://127.0.0.1:8082",
		1,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, writeErr := w.Write([]byte("tenant-b")); writeErr != nil {
				return
			}
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	serviceA := valeruntime.NewService("tenant-a", "round_robin", tenantA)
	serviceB := valeruntime.NewService("tenant-b", "round_robin", tenantB)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":0", valeruntime.EntrypointRuntime{Name: "web", Address: ":0"}).
		AddService(serviceA).
		AddService(serviceB).
		AddRoute(
			valeruntime.NewRoute("tenant-a", "web", serviceA).
				WithHost("api.example.com").
				WithPathPrefix("/api").
				WithMethod(http.MethodGet).
				WithHeader("X-Tenant", "a"),
		).
		AddRoute(
			valeruntime.NewRoute("tenant-b", "web", serviceB).
				WithHost("api.example.com").
				WithPathPrefix("/api").
				WithMethod(http.MethodGet).
				WithHeader("X-Tenant", "b"),
		)
	for idx := range 20 {
		snapshot.AddRoute(
			valeruntime.NewRoute("filler-"+strconv.Itoa(idx), "web", serviceA).
				WithHost("api.example.com").
				WithPathPrefix("/filler/" + strconv.Itoa(idx)).
				WithMethod(http.MethodGet),
		)
	}
	handler := valeruntime.NewGateway(snapshot.BuildMatchers(), nil, false, valeruntime.NewNoopMetrics()).Handler("web")

	for _, tt := range []struct {
		tenant string
		want   string
	}{
		{tenant: "a", want: "tenant-a"},
		{tenant: "b", want: "tenant-b"},
	} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com/api/users", http.NoBody)
		req.Header.Set("X-Tenant", tt.tenant)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		if recorder.Body.String() != tt.want {
			t.Fatalf("tenant %q response = %q, want %q", tt.tenant, recorder.Body.String(), tt.want)
		}
	}
}

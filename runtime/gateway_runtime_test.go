package runtime_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestGatewayLogsFailedRequestWhenAccessLogDisabled(t *testing.T) {
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

	valeruntime.NewGateway(snapshot, logger, false, valeruntime.NewNoopMetrics()).
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

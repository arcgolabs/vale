package runtime_test

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestHealthCheckerKeepsInitialUnhealthyTransitionBelowInfo(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	endpoint := healthLogEndpoint(t, closedLocalURL(t))
	gateway := healthLogGateway(endpoint)
	checker := valeruntime.NewHealthCheckerWithLogger(time.Millisecond, time.Second, logger)
	checker.Start(t.Context(), gateway)
	defer checker.Stop()

	waitForHealthState(t, endpoint, false)

	if logs.Len() != 0 {
		t.Fatalf("initial unhealthy logs = %q, want no info-level output", logs.String())
	}
}

func TestHealthCheckerLogsPostRecoveryUnhealthyTransitionAtWarn(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	endpoint := healthLogEndpoint(t, server.URL)
	gateway := healthLogGateway(endpoint)
	checker := valeruntime.NewHealthCheckerWithLogger(time.Millisecond, time.Second, logger)
	checker.Start(t.Context(), gateway)
	defer checker.Stop()

	waitForHealthCheck(t, endpoint)
	server.Close()
	waitForLog(t, &logs, "level=WARN")

	output := logs.String()
	if !strings.Contains(output, "endpoint health changed") {
		t.Fatalf("post-recovery unhealthy logs = %q, want warning health change", output)
	}
}

func healthLogEndpoint(t *testing.T, rawURL string) *valeruntime.EndpointRuntime {
	t.Helper()

	endpoint, err := valeruntime.NewEndpoint(rawURL, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

func healthLogGateway(endpoint *valeruntime.EndpointRuntime) *valeruntime.Gateway {
	endpoints := collectionlist.NewList(endpoint)
	service := &valeruntime.ServiceRuntime{
		Name:      "api",
		Strategy:  "round_robin",
		Endpoints: endpoints,
	}
	service.BuildSlots()
	return valeruntime.NewGateway(valeruntime.NewSnapshot().AddService(service), nil, false, valeruntime.NewNoopMetrics())
}

func closedLocalURL(t *testing.T) string {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + addr
}

func waitForHealthState(t *testing.T, endpoint *valeruntime.EndpointRuntime, healthy bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if endpoint.Healthy.Load() == healthy {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("endpoint health = %v, want %v", endpoint.Healthy.Load(), healthy)
}

func waitForHealthCheck(t *testing.T, endpoint *valeruntime.EndpointRuntime) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if endpoint.LastChecked.Load() > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("endpoint was not health checked")
}

func waitForLog(t *testing.T, logs *bytes.Buffer, pattern string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(logs.String(), pattern) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("logs = %q, want pattern %q", logs.String(), pattern)
}

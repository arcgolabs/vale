package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

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

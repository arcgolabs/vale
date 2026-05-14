package gateway_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arcgolabs/vale/gateway"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestReloadRestartKeepsHealthCheckerRunning(t *testing.T) {
	t.Parallel()

	var healthRequests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		healthRequests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	initial := testGatewaySnapshot(t, upstream.URL, "1h")
	reloaded := testGatewaySnapshot(t, upstream.URL, "10ms")
	reloaded.Entrypoints = initial.Entrypoints
	reloaded.EntrypointConfigs = initial.EntrypointConfigs
	reloaded.AdminAddress = initial.AdminAddress

	provider := &reloadTestProvider{snapshot: initial}
	g, err := gateway.New(
		gateway.WithSnapshotProvider(provider),
		gateway.WithWatch(true),
		gateway.WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer stopGateway(t, g)

	provider.Reload(reloaded)

	deadline := time.Now().Add(2 * time.Second)
	for healthRequests.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if healthRequests.Load() == 0 {
		t.Fatal("health checker did not run after restart reload")
	}
}

type reloadTestProvider struct {
	snapshot *valeruntime.CompiledSnapshot
	reload   func(*valeruntime.CompiledSnapshot)
}

func (p *reloadTestProvider) Load(context.Context) (*valeruntime.CompiledSnapshot, error) {
	return p.snapshot, nil
}

func (p *reloadTestProvider) Watch(_ context.Context, onReload func(*valeruntime.CompiledSnapshot), _ func(error)) (io.Closer, error) {
	p.reload = onReload
	return nopReloadCloser{}, nil
}

func (p *reloadTestProvider) Reload(snapshot *valeruntime.CompiledSnapshot) {
	p.reload(snapshot)
}

type nopReloadCloser struct{}

func (nopReloadCloser) Close() error {
	return nil
}

func testGatewaySnapshot(t *testing.T, endpointURL, healthInterval string) *valeruntime.CompiledSnapshot {
	t.Helper()

	endpoint, err := valeruntime.NewEndpoint(endpointURL, 1, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	service := valeruntime.NewService("test", "round_robin", endpoint)
	route := valeruntime.NewRoute("test", "web", service).WithPathPrefix("/")
	entrypointAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", entrypointAddr, valeruntime.EntrypointRuntime{
			Name:    "web",
			Address: entrypointAddr,
		}).
		AddService(service).
		AddRoute(route).
		BuildMatchers()
	snapshot.AdminAddress = adminAddr
	snapshot.HealthInterval = healthInterval
	snapshot.HealthTimeout = "500ms"
	snapshot.Security = valeruntime.SecurityRuntime{
		ReadHeaderTimeout: "5s",
		ReadTimeout:       "30s",
		WriteTimeout:      "30s",
		IdleTimeout:       "120s",
		MaxHeaderBytes:    1 << 20,
		MaxBodyBytes:      32 << 20,
	}
	return snapshot
}

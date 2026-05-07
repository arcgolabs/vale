package runtime

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func TestServiceRuntimePickSkipsUnhealthyEndpoints(t *testing.T) {
	t.Parallel()

	healthyURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	unhealthyURL, err := url.Parse("http://127.0.0.1:8082")
	if err != nil {
		t.Fatal(err)
	}

	service := &ServiceRuntime{
		Name:     "api",
		Strategy: "round_robin",
		Endpoints: collectionlist.NewList[*EndpointRuntime](
			&EndpointRuntime{URL: unhealthyURL, Weight: 1},
			&EndpointRuntime{URL: healthyURL, Weight: 1},
		),
	}
	unhealthy, _ := service.Endpoints.Get(0)
	healthy, _ := service.Endpoints.Get(1)
	unhealthy.Healthy.Store(false)
	healthy.Healthy.Store(true)

	for range 4 {
		got, err := service.Pick()
		if err != nil {
			t.Fatal(err)
		}
		if got.URL.String() != healthyURL.String() {
			t.Fatalf("picked endpoint = %s, want %s", got.URL.String(), healthyURL.String())
		}
	}
}

func TestServiceRuntimeWeightedRoundRobinUsesWeights(t *testing.T) {
	t.Parallel()

	firstURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	secondURL, err := url.Parse("http://127.0.0.1:8082")
	if err != nil {
		t.Fatal(err)
	}

	service := &ServiceRuntime{
		Name:     "api",
		Strategy: "weighted_round_robin",
		Endpoints: collectionlist.NewList[*EndpointRuntime](
			&EndpointRuntime{URL: firstURL, Weight: 2},
			&EndpointRuntime{URL: secondURL, Weight: 1},
		),
	}
	service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
		endpoint.Healthy.Store(true)
		return true
	})
	service.BuildSlots()

	counts := map[string]int{}
	for range 6 {
		got, err := service.Pick()
		if err != nil {
			t.Fatal(err)
		}
		counts[got.URL.String()]++
	}

	if counts[firstURL.String()] != 4 {
		t.Fatalf("first endpoint picks = %d, want 4", counts[firstURL.String()])
	}
	if counts[secondURL.String()] != 2 {
		t.Fatalf("second endpoint picks = %d, want 2", counts[secondURL.String()])
	}
}

func TestGatewayInvokesExtendedMetricsRecorder(t *testing.T) {
	t.Parallel()

	metrics := &testExtendedMetricsRecorder{}
	gateway := NewGateway(NewSnapshot(), nil, false, metrics)
	if metrics.snapshots != 1 {
		t.Fatalf("snapshots = %d, want 1 after NewGateway", metrics.snapshots)
	}

	gateway.Swap(NewSnapshot())
	if metrics.snapshots != 2 {
		t.Fatalf("snapshots = %d, want 2 after Swap", metrics.snapshots)
	}

	endpointURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &EndpointRuntime{URL: endpointURL}
	gateway.ObserveHealth(endpoint, true)
	if metrics.healthChecks != 1 {
		t.Fatalf("health checks = %d, want 1", metrics.healthChecks)
	}

	gateway.ObserveReload("swapped")
	if metrics.reloads != 1 || metrics.lastReloadResult != "swapped" {
		t.Fatalf("reloads = %d result = %q, want one swapped reload", metrics.reloads, metrics.lastReloadResult)
	}
}

type testExtendedMetricsRecorder struct {
	snapshots        int
	reloads          int
	healthChecks     int
	lastReloadResult string
}

func (r *testExtendedMetricsRecorder) Observe(_ *CompiledRoute, _ *EndpointRuntime, _ int, _ time.Duration) {
}

func (r *testExtendedMetricsRecorder) Handler() http.Handler {
	return http.NotFoundHandler()
}

func (r *testExtendedMetricsRecorder) ObserveSnapshot(_ *CompiledSnapshot) {
	r.snapshots++
}

func (r *testExtendedMetricsRecorder) ObserveReload(result string) {
	r.reloads++
	r.lastReloadResult = result
}

func (r *testExtendedMetricsRecorder) ObserveHealth(_ *EndpointRuntime, _ bool) {
	r.healthChecks++
}

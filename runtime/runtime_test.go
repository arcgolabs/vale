package runtime_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
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

	service := &valeruntime.ServiceRuntime{
		Name:     "api",
		Strategy: "round_robin",
		Endpoints: collectionlist.NewList[*valeruntime.EndpointRuntime](
			&valeruntime.EndpointRuntime{URL: unhealthyURL, Weight: 1},
			&valeruntime.EndpointRuntime{URL: healthyURL, Weight: 1},
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

func TestServiceRuntimePickSingleEndpoint(t *testing.T) {
	t.Parallel()

	endpointURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &valeruntime.EndpointRuntime{URL: endpointURL, Weight: 1}
	endpoint.Healthy.Store(true)
	service := &valeruntime.ServiceRuntime{
		Name:      "api",
		Strategy:  "round_robin",
		Endpoints: collectionlist.NewList(endpoint),
	}

	for range 4 {
		got, err := service.Pick()
		if err != nil {
			t.Fatal(err)
		}
		if got != endpoint {
			t.Fatalf("picked endpoint = %v, want single endpoint", got)
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

	service := &valeruntime.ServiceRuntime{
		Name:     "api",
		Strategy: "weighted_round_robin",
		Endpoints: collectionlist.NewList[*valeruntime.EndpointRuntime](
			&valeruntime.EndpointRuntime{URL: firstURL, Weight: 2},
			&valeruntime.EndpointRuntime{URL: secondURL, Weight: 1},
		),
	}
	service.Endpoints.Range(func(_ int, endpoint *valeruntime.EndpointRuntime) bool {
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
	gateway := valeruntime.NewGateway(valeruntime.NewSnapshot(), nil, false, metrics)
	assertMetricCount(t, "snapshots after NewGateway", metrics.snapshots, 1)

	gateway.Swap(valeruntime.NewSnapshot())
	assertMetricCount(t, "snapshots after Swap", metrics.snapshots, 2)

	observeExtendedMetrics(t, gateway)
	assertExtendedMetricCounts(t, metrics)
}

func observeExtendedMetrics(t *testing.T, gateway *valeruntime.Gateway) {
	t.Helper()

	endpointURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &valeruntime.EndpointRuntime{URL: endpointURL}
	gateway.ObserveHealth(endpoint, true)
	gateway.ObserveHealthCheck(endpoint, true, time.Millisecond)
	gateway.ObserveRouteCache(true)
	gateway.ObserveRouteCache(false)
	gateway.ObserveReloadDuration("swapped", 100*time.Millisecond)
	gateway.ObserveReloadDebounce(100*time.Millisecond, 2)
	gateway.ObserveRaftApply("data", 50*time.Millisecond, "success")
	gateway.ObserveReload("swapped")
}

func assertExtendedMetricCounts(t *testing.T, metrics *testExtendedMetricsRecorder) {
	t.Helper()

	assertMetricCount(t, "health checks", metrics.healthChecks, 1)
	assertMetricCount(t, "health check durations", metrics.healthCheckDurations, 1)
	assertMetricCount(t, "route cache hits", metrics.routeCacheHits, 1)
	assertMetricCount(t, "route cache misses", metrics.routeCacheMisses, 1)
	assertMetricCount(t, "reload durations", metrics.reloadDurations, 1)
	assertMetricCount(t, "reload debounces", metrics.reloadDebounces, 1)
	assertMetricCount(t, "raft applys", metrics.raftApplys, 1)
	assertMetricCount(t, "raft apply successes", metrics.raftApplySuccess, 1)
	assertMetricCount(t, "reloads", metrics.reloads, 1)
	if metrics.lastReloadResult != "swapped" {
		t.Fatalf("reload result = %q, want swapped", metrics.lastReloadResult)
	}
}

func assertMetricCount(t *testing.T, name string, got, want int) {
	t.Helper()

	if got != want {
		t.Fatalf("%s = %d, want %d", name, got, want)
	}
}

type testExtendedMetricsRecorder struct {
	snapshots            int
	reloads              int
	healthChecks         int
	healthCheckDurations int
	routeCacheHits       int
	routeCacheMisses     int
	reloadDurations      int
	reloadDebounces      int
	raftApplys           int
	raftApplySuccess     int
	lastReloadResult     string
}

func (r *testExtendedMetricsRecorder) Observe(_ *valeruntime.CompiledRoute, _ *valeruntime.EndpointRuntime, _ int, _ time.Duration) {
}

func (r *testExtendedMetricsRecorder) Handler() http.Handler {
	return http.NotFoundHandler()
}

func (r *testExtendedMetricsRecorder) ObserveSnapshot(_ *valeruntime.CompiledSnapshot) {
	r.snapshots++
}

func (r *testExtendedMetricsRecorder) ObserveReload(result string) {
	r.reloads++
	r.lastReloadResult = result
}

func (r *testExtendedMetricsRecorder) ObserveHealth(_ *valeruntime.EndpointRuntime, _ bool) {
	r.healthChecks++
}

func (r *testExtendedMetricsRecorder) ObserveHealthCheck(_ *valeruntime.EndpointRuntime, _ bool, _ time.Duration) {
	r.healthCheckDurations++
}

func (r *testExtendedMetricsRecorder) ObserveRouteCache(hit bool) {
	if hit {
		r.routeCacheHits++
		return
	}
	r.routeCacheMisses++
}

func (r *testExtendedMetricsRecorder) ObserveReloadDuration(_ string, _ time.Duration) {
	r.reloadDurations++
}

func (r *testExtendedMetricsRecorder) ObserveReloadDebounce(_ time.Duration, _ int) {
	r.reloadDebounces++
}

func (r *testExtendedMetricsRecorder) ObserveRaftApply(_ string, _ time.Duration, result string) {
	r.raftApplys++
	if result == "success" {
		r.raftApplySuccess++
	}
}

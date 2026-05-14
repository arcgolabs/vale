package runtime

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/arcgolabs/observabilityx"
)

type MetricsRecorder interface {
	Observe(route *CompiledRoute, endpoint *EndpointRuntime, status int, duration time.Duration)
	Handler() http.Handler
}

type noopMetrics struct{}

func NewNoopMetrics() MetricsRecorder {
	return noopMetrics{}
}

func (noopMetrics) Enabled() bool {
	return false
}

func (noopMetrics) Observe(_ *CompiledRoute, _ *EndpointRuntime, _ int, _ time.Duration) {}

func (noopMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "metrics unavailable", http.StatusNotFound)
	})
}

type observabilityMetrics struct {
	enabled        bool
	requests       observabilityx.Counter
	latency        observabilityx.Histogram
	reloads        observabilityx.Counter
	reloadDelay    observabilityx.Histogram
	reloadDebounce observabilityx.Histogram
	healthChecks   observabilityx.Counter
	healthLatency  observabilityx.Histogram
	routeCache     observabilityx.Counter
	routes         observabilityx.Gauge
	services       observabilityx.Gauge
	endpoints      observabilityx.Gauge
	raftApply      observabilityx.Histogram
	raftApplyOps   observabilityx.Counter
	handler        http.Handler
}

type metricsHandler interface {
	Handler() http.Handler
}

func NewObservabilityMetrics(enabled bool, obs observabilityx.Observability, handler http.Handler) MetricsRecorder {
	if obs == nil {
		obs = observabilityx.Nop()
	}
	if handler == nil {
		if h, ok := obs.(metricsHandler); ok {
			handler = h.Handler()
		}
	}
	if handler == nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "metrics unavailable", http.StatusNotFound)
		})
	}
	return &observabilityMetrics{
		enabled: enabled,
		requests: obs.Counter(observabilityx.NewCounterSpec("http_requests_total",
			observabilityx.WithDescription("Total HTTP requests handled by vale."),
			observabilityx.WithLabelKeys("route", "service", "endpoint", "status"),
		)),
		latency: obs.Histogram(observabilityx.NewHistogramSpec("http_request_duration_seconds",
			observabilityx.WithDescription("HTTP request duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("route", "service"),
		)),
		reloads: obs.Counter(observabilityx.NewCounterSpec("runtime_reloads_total",
			observabilityx.WithDescription("Total runtime reload attempts."),
			observabilityx.WithLabelKeys("result"),
		)),
		reloadDelay: obs.Histogram(observabilityx.NewHistogramSpec("runtime_reload_duration_seconds",
			observabilityx.WithDescription("Runtime reload duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("result"),
		)),
		reloadDebounce: obs.Histogram(observabilityx.NewHistogramSpec("runtime_reload_debounce_delay_seconds",
			observabilityx.WithDescription("Delay between config change and debounced reload trigger."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("source_count"),
		)),
		healthChecks: obs.Counter(observabilityx.NewCounterSpec("health_checks_total",
			observabilityx.WithDescription("Total endpoint health check results."),
			observabilityx.WithLabelKeys("endpoint", "healthy"),
		)),
		healthLatency: obs.Histogram(observabilityx.NewHistogramSpec("health_check_duration_seconds",
			observabilityx.WithDescription("Endpoint health check duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("endpoint", "healthy"),
		)),
		routeCache: obs.Counter(observabilityx.NewCounterSpec("route_match_cache_total",
			observabilityx.WithDescription("Total route match cache lookups."),
			observabilityx.WithLabelKeys("result"),
		)),
		raftApply: obs.Histogram(observabilityx.NewHistogramSpec("runtime_raft_apply_duration_seconds",
			observabilityx.WithDescription("Raft apply duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("group", "result"),
		)),
		raftApplyOps: obs.Counter(observabilityx.NewCounterSpec("runtime_raft_apply_total",
			observabilityx.WithDescription("Total raft apply attempts."),
			observabilityx.WithLabelKeys("group", "result"),
		)),
		routes: obs.Gauge(observabilityx.NewGaugeSpec("active_routes",
			observabilityx.WithDescription("Current compiled route count."),
		)),
		services: obs.Gauge(observabilityx.NewGaugeSpec("active_services",
			observabilityx.WithDescription("Current compiled service count."),
		)),
		endpoints: obs.Gauge(observabilityx.NewGaugeSpec("active_endpoints",
			observabilityx.WithDescription("Current compiled endpoint count."),
		)),
		handler: handler,
	}
}

func (m *observabilityMetrics) Observe(route *CompiledRoute, endpoint *EndpointRuntime, status int, duration time.Duration) {
	if !m.enabled {
		return
	}
	ctx := context.Background()
	m.requests.Add(ctx, 1,
		observabilityx.String("route", route.Name),
		observabilityx.String("service", route.Service.Name),
		observabilityx.String("endpoint", endpoint.URL.String()),
		observabilityx.String("status", strconv.Itoa(status)),
	)
	m.latency.Record(ctx, duration.Seconds(),
		observabilityx.String("route", route.Name),
		observabilityx.String("service", route.Service.Name),
	)
}

func (m *observabilityMetrics) Enabled() bool {
	return m != nil && m.enabled
}

func (m *observabilityMetrics) Handler() http.Handler {
	if !m.enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "metrics disabled", http.StatusNotFound)
		})
	}
	return m.handler
}

type SnapshotMetricsRecorder interface {
	ObserveSnapshot(snapshot *CompiledSnapshot)
}

type ReloadMetricsRecorder interface {
	ObserveReload(result string)
}

type ReloadDurationMetricsRecorder interface {
	ObserveReloadDuration(result string, duration time.Duration)
}

type ReloadDebounceMetricsRecorder interface {
	ObserveReloadDebounce(delay time.Duration, sourceCount int)
}

type HealthMetricsRecorder interface {
	ObserveHealth(endpoint *EndpointRuntime, healthy bool)
}

type HealthCheckMetricsRecorder interface {
	ObserveHealthCheck(endpoint *EndpointRuntime, healthy bool, duration time.Duration)
}

type RouteCacheMetricsRecorder interface {
	ObserveRouteCache(hit bool)
}

type RaftApplyMetricsRecorder interface {
	ObserveRaftApply(group string, duration time.Duration, result string)
}

func (g *Gateway) ObserveSnapshot(snapshot *CompiledSnapshot) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(SnapshotMetricsRecorder); ok {
		recorder.ObserveSnapshot(snapshot)
	}
}

func (g *Gateway) ObserveReload(result string) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(ReloadMetricsRecorder); ok {
		recorder.ObserveReload(result)
	}
}

func (g *Gateway) ObserveReloadDuration(result string, duration time.Duration) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(ReloadDurationMetricsRecorder); ok {
		recorder.ObserveReloadDuration(result, duration)
	}
}

func (g *Gateway) ObserveReloadDebounce(delay time.Duration, sourceCount int) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(ReloadDebounceMetricsRecorder); ok {
		recorder.ObserveReloadDebounce(delay, sourceCount)
	}
}

func (g *Gateway) ObserveHealth(endpoint *EndpointRuntime, healthy bool) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(HealthMetricsRecorder); ok {
		recorder.ObserveHealth(endpoint, healthy)
	}
}

func (g *Gateway) ObserveHealthCheck(endpoint *EndpointRuntime, healthy bool, duration time.Duration) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(HealthCheckMetricsRecorder); ok {
		recorder.ObserveHealthCheck(endpoint, healthy, duration)
	}
}

func (g *Gateway) ObserveRouteCache(hit bool) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(RouteCacheMetricsRecorder); ok {
		recorder.ObserveRouteCache(hit)
	}
}

func (g *Gateway) ObserveRaftApply(group string, duration time.Duration, result string) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(RaftApplyMetricsRecorder); ok {
		recorder.ObserveRaftApply(group, duration, result)
	}
}

func (m *observabilityMetrics) ObserveSnapshot(snapshot *CompiledSnapshot) {
	if !m.enabled || snapshot == nil {
		return
	}
	ctx := context.Background()
	m.routes.Set(ctx, float64(snapshot.Routes().Len()))
	m.services.Set(ctx, float64(snapshot.Services.Len()))
	endpointCount := 0
	snapshot.Services.Range(func(_ string, service *ServiceRuntime) bool {
		endpointCount += service.Endpoints.Len()
		return true
	})
	m.endpoints.Set(ctx, float64(endpointCount))
}

func (m *observabilityMetrics) ObserveReload(result string) {
	if !m.enabled {
		return
	}
	m.reloads.Add(context.Background(), 1, observabilityx.String("result", result))
}

func (m *observabilityMetrics) ObserveReloadDuration(result string, duration time.Duration) {
	if !m.enabled {
		return
	}
	m.reloadDelay.Record(
		context.Background(),
		duration.Seconds(),
		observabilityx.String("result", result),
	)
}

func (m *observabilityMetrics) ObserveReloadDebounce(delay time.Duration, sourceCount int) {
	if !m.enabled {
		return
	}
	m.reloadDebounce.Record(
		context.Background(),
		delay.Seconds(),
		observabilityx.String("source_count", strconv.Itoa(sourceCount)),
	)
}

func (m *observabilityMetrics) ObserveHealth(endpoint *EndpointRuntime, healthy bool) {
	if !m.enabled || endpoint == nil || endpoint.URL == nil {
		return
	}
	m.healthChecks.Add(context.Background(), 1,
		observabilityx.String("endpoint", endpoint.URL.String()),
		observabilityx.String("healthy", strconv.FormatBool(healthy)),
	)
}

func (m *observabilityMetrics) ObserveHealthCheck(endpoint *EndpointRuntime, healthy bool, duration time.Duration) {
	if !m.enabled || endpoint == nil || endpoint.URL == nil {
		return
	}
	m.healthLatency.Record(context.Background(), duration.Seconds(),
		observabilityx.String("endpoint", endpoint.URL.String()),
		observabilityx.String("healthy", strconv.FormatBool(healthy)),
	)
}

func (m *observabilityMetrics) ObserveRouteCache(hit bool) {
	if !m.enabled {
		return
	}
	result := "miss"
	if hit {
		result = "hit"
	}
	m.routeCache.Add(context.Background(), 1, observabilityx.String("result", result))
}

func (m *observabilityMetrics) ObserveRaftApply(group string, duration time.Duration, result string) {
	if !m.enabled {
		return
	}
	m.raftApply.Record(
		context.Background(),
		duration.Seconds(),
		observabilityx.String("group", group),
		observabilityx.String("result", result),
	)
	m.raftApplyOps.Add(
		context.Background(),
		1,
		observabilityx.String("group", group),
		observabilityx.String("result", result),
	)
}

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

func (noopMetrics) Observe(_ *CompiledRoute, _ *EndpointRuntime, _ int, _ time.Duration) {}

func (noopMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "metrics unavailable", http.StatusNotFound)
	})
}

type observabilityMetrics struct {
	enabled      bool
	requests     observabilityx.Counter
	latency      observabilityx.Histogram
	reloads      observabilityx.Counter
	healthChecks observabilityx.Counter
	routes       observabilityx.Gauge
	services     observabilityx.Gauge
	endpoints    observabilityx.Gauge
	handler      http.Handler
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
		requests: obs.Counter(observabilityx.NewCounterSpec("vale_http_requests_total",
			observabilityx.WithDescription("Total HTTP requests handled by vale."),
			observabilityx.WithLabelKeys("route", "service", "endpoint", "status"),
		)),
		latency: obs.Histogram(observabilityx.NewHistogramSpec("vale_http_request_duration_seconds",
			observabilityx.WithDescription("HTTP request duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("route", "service"),
		)),
		reloads: obs.Counter(observabilityx.NewCounterSpec("vale_runtime_reloads_total",
			observabilityx.WithDescription("Total runtime reload attempts."),
			observabilityx.WithLabelKeys("result"),
		)),
		healthChecks: obs.Counter(observabilityx.NewCounterSpec("vale_health_checks_total",
			observabilityx.WithDescription("Total endpoint health check results."),
			observabilityx.WithLabelKeys("endpoint", "healthy"),
		)),
		routes: obs.Gauge(observabilityx.NewGaugeSpec("vale_active_routes",
			observabilityx.WithDescription("Current compiled route count."),
		)),
		services: obs.Gauge(observabilityx.NewGaugeSpec("vale_active_services",
			observabilityx.WithDescription("Current compiled service count."),
		)),
		endpoints: obs.Gauge(observabilityx.NewGaugeSpec("vale_active_endpoints",
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

type HealthMetricsRecorder interface {
	ObserveHealth(endpoint *EndpointRuntime, healthy bool)
}

func (g *Gateway) ObserveSnapshot(snapshot *CompiledSnapshot) {
	if recorder, ok := g.metrics.(SnapshotMetricsRecorder); ok {
		recorder.ObserveSnapshot(snapshot)
	}
}

func (g *Gateway) ObserveReload(result string) {
	if recorder, ok := g.metrics.(ReloadMetricsRecorder); ok {
		recorder.ObserveReload(result)
	}
}

func (g *Gateway) ObserveHealth(endpoint *EndpointRuntime, healthy bool) {
	if recorder, ok := g.metrics.(HealthMetricsRecorder); ok {
		recorder.ObserveHealth(endpoint, healthy)
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

func (m *observabilityMetrics) ObserveHealth(endpoint *EndpointRuntime, healthy bool) {
	if !m.enabled || endpoint == nil || endpoint.URL == nil {
		return
	}
	m.healthChecks.Add(context.Background(), 1,
		observabilityx.String("endpoint", endpoint.URL.String()),
		observabilityx.String("healthy", strconv.FormatBool(healthy)),
	)
}

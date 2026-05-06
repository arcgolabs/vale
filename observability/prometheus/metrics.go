package prometheus

import (
	"fmt"
	"net/http"
	"time"

	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/runtime"
	prom "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	enabled  bool
	registry *prom.Registry
	requests *prom.CounterVec
	latency  *prom.HistogramVec
}

func New(enabled bool) runtime.MetricsRecorder {
	metrics := &Metrics{
		enabled: enabled,
	}
	if !enabled {
		return metrics
	}

	metrics.registry = prom.NewRegistry()
	metrics.requests = prom.NewCounterVec(
		prom.CounterOpts{
			Name: "vela_http_requests_total",
			Help: "Total HTTP requests handled by vela.",
		},
		[]string{"route", "service", "endpoint", "status"},
	)
	metrics.latency = prom.NewHistogramVec(
		prom.HistogramOpts{
			Name:    "vela_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prom.DefBuckets,
		},
		[]string{"route", "service"},
	)
	if err := metrics.registry.Register(metrics.requests); err != nil {
		panic(err)
	}
	if err := metrics.registry.Register(metrics.latency); err != nil {
		panic(err)
	}
	return metrics
}

func WithMetrics() gateway.Option {
	return gateway.WithMetricsFactory(New)
}

func (m *Metrics) Observe(route *runtime.CompiledRoute, endpoint *runtime.EndpointRuntime, status int, duration time.Duration) {
	if !m.enabled {
		return
	}
	m.requests.WithLabelValues(route.Name, route.Service.Name, endpoint.URL.String(), fmt.Sprintf("%d", status)).Inc()
	m.latency.WithLabelValues(route.Name, route.Service.Name).Observe(duration.Seconds())
}

func (m *Metrics) Handler() http.Handler {
	if !m.enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "metrics disabled", http.StatusNotFound)
		})
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

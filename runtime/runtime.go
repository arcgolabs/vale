package runtime

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/observabilityx"
	"github.com/samber/oops"
)

type CompiledSnapshot struct {
	Entrypoints        *mapping.Map[string, string]
	EntrypointConfigs  *mapping.Map[string, EntrypointRuntime]
	RoutesByEntrypoint *mapping.MultiMap[string, *CompiledRoute]
	EntrypointMatchers *mapping.Map[string, *EntrypointMatcher]
	Catalog            *Catalog
	Services           *mapping.Map[string, *ServiceRuntime]
	AdminAddress       string
	AccessLogEnabled   bool
	MetricsEnabled     bool
	HealthInterval     string
	HealthTimeout      string
	Security           SecurityRuntime
	ProxyEngine        string
	BuiltAt            time.Time
}

type CompiledRoute struct {
	Name        string
	Entrypoint  string
	Host        string
	PathPrefix  string
	Method      string
	Headers     *mapping.Map[string, string]
	Service     *ServiceRuntime
	Predicates  *bitset.BitSet
	Middlewares *collectionlist.List[MiddlewareRuntime]
}

type EntrypointRuntime struct {
	Name    string
	Address string
	TLS     TLSRuntime
}

type TLSRuntime struct {
	Enabled  bool
	CertFile string
	KeyFile  string
	ACME     ACMERuntime
}

type ACMERuntime struct {
	Enabled  bool
	Email    string
	CacheDir string
	Domains  *collectionlist.List[string]
}

type SecurityRuntime struct {
	ReadHeaderTimeout string
	ReadTimeout       string
	WriteTimeout      string
	IdleTimeout       string
	MaxHeaderBytes    int
	MaxBodyBytes      int64
}

type MiddlewareRuntime struct {
	Name            string
	Type            string
	StripPrefix     string
	AddPrefix       string
	RequestHeaders  *mapping.Map[string, string]
	ResponseHeaders *mapping.Map[string, string]
	MaxBodyBytes    int64
}

type ServiceRuntime struct {
	Name          string
	Strategy      string
	Endpoints     *collectionlist.List[*EndpointRuntime]
	weightedSlots *collectionlist.List[int]
	rrCounter     atomic.Uint64
}

func (s *ServiceRuntime) BuildSlots() {
	s.weightedSlots = collectionlist.NewList[int]()
	if s.Strategy != "weighted_round_robin" {
		return
	}
	if s.Endpoints == nil {
		return
	}
	s.Endpoints.Range(func(idx int, endpoint *EndpointRuntime) bool {
		weight := endpoint.Weight
		if weight <= 0 {
			weight = 1
		}
		for range weight {
			s.weightedSlots.Add(idx)
		}
		return true
	})
}

type EndpointRuntime struct {
	URL         *url.URL
	Weight      int
	Proxy       http.Handler
	Healthy     atomic.Bool
	LastChecked atomic.Int64
}

func (s *ServiceRuntime) Pick() (*EndpointRuntime, error) {
	endpointCount := s.Endpoints.Len()
	if endpointCount == 0 {
		return nil, oops.
			In("runtime").
			With("service", s.Name).
			New("service has no endpoints")
	}
	if s.Strategy == "weighted_round_robin" && !s.weightedSlots.IsEmpty() {
		if endpoint := s.pickWeightedEndpoint(); endpoint != nil {
			return endpoint, nil
		}
	} else if endpoint := s.pickRoundRobinEndpoint(endpointCount); endpoint != nil {
		return endpoint, nil
	}
	return nil, oops.
		In("runtime").
		With("service", s.Name, "endpoints", endpointCount, "strategy", s.Strategy).
		New("no healthy endpoint")
}

func (s *ServiceRuntime) pickWeightedEndpoint() *EndpointRuntime {
	slotCount := s.weightedSlots.Len()
	//nolint:gosec // The modulo result is strictly smaller than slotCount before converting to int.
	start := int(s.rrCounter.Add(1) % uint64(slotCount))
	for offset := range slotCount {
		slot, _ := s.weightedSlots.Get((start + offset) % slotCount)
		endpoint, _ := s.Endpoints.Get(slot)
		if endpoint.Healthy.Load() {
			return endpoint
		}
	}
	return nil
}

func (s *ServiceRuntime) pickRoundRobinEndpoint(endpointCount int) *EndpointRuntime {
	//nolint:gosec // The modulo result is strictly smaller than endpointCount before converting to int.
	start := int(s.rrCounter.Add(1) % uint64(endpointCount))
	for offset := range endpointCount {
		endpoint, _ := s.Endpoints.Get((start + offset) % endpointCount)
		if endpoint.Healthy.Load() {
			return endpoint
		}
	}
	return nil
}

type Gateway struct {
	current            atomic.Pointer[CompiledSnapshot]
	access             *AccessLogger
	metrics            MetricsRecorder
	middlewareRegistry *MiddlewareRegistry
}

func NewGateway(snapshot *CompiledSnapshot, logger *slog.Logger, accessEnabled bool, metrics MetricsRecorder) *Gateway {
	return NewGatewayWithMiddlewareRegistry(snapshot, logger, accessEnabled, metrics, nil)
}

func NewGatewayWithMiddlewareRegistry(snapshot *CompiledSnapshot, logger *slog.Logger, accessEnabled bool, metrics MetricsRecorder, registry *MiddlewareRegistry) *Gateway {
	accessLogger := NewAccessLogger(logger, accessEnabled)
	if metrics == nil {
		metrics = NewNoopMetrics()
	}
	if registry == nil {
		registry = DefaultMiddlewareRegistry()
	}
	gateway := &Gateway{
		access:             accessLogger,
		metrics:            metrics,
		middlewareRegistry: registry,
	}
	gateway.current.Store(snapshot)
	gateway.ObserveSnapshot(snapshot)
	return gateway
}

func (g *Gateway) Swap(snapshot *CompiledSnapshot) {
	g.current.Store(snapshot)
	g.ObserveSnapshot(snapshot)
}

func (g *Gateway) Snapshot() *CompiledSnapshot {
	return g.current.Load()
}

func (g *Gateway) MetricsHandler() http.Handler {
	return g.metrics.Handler()
}

func (g *Gateway) Handler(entrypoint string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		snapshot := g.current.Load()
		if snapshot == nil {
			http.Error(w, "runtime not ready", http.StatusServiceUnavailable)
			return
		}

		matcher, _ := snapshot.EntrypointMatchers.Get(entrypoint)
		routes := snapshot.RoutesByEntrypoint.Get(entrypoint)
		route := MatchRoute(matcher, routes, r)
		if route == nil {
			http.NotFound(w, r)
			return
		}

		endpoint, err := route.Service.Pick()
		if err != nil {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}

		start := time.Now()
		recorder := newStatusRecorder(w)
		handler := WrapMiddlewaresWithRegistry(endpoint.Proxy, route.Middlewares, g.middlewareRegistry)
		if snapshot.Security.MaxBodyBytes > 0 {
			r.Body = http.MaxBytesReader(recorder, r.Body, snapshot.Security.MaxBodyBytes)
		}
		handler.ServeHTTP(recorder, r)
		duration := time.Since(start)

		g.metrics.Observe(route, endpoint, recorder.status, duration)
		g.access.Log(AccessEvent{
			Method:     r.Method,
			Path:       r.URL.Path,
			Host:       r.Host,
			StatusCode: recorder.status,
			DurationMs: duration.Milliseconds(),
			Route:      route.Name,
			Service:    route.Service.Name,
			Endpoint:   endpoint.URL.String(),
			UserAgent:  r.UserAgent(),
			RemoteAddr: r.RemoteAddr,
		})
	})
}

func matchHeaders(expected *mapping.Map[string, string], actual http.Header) bool {
	if expected == nil {
		return true
	}
	matched := true
	expected.Range(func(key string, expectedValue string) bool {
		values := actual.Values(key)
		if len(values) == 0 {
			matched = false
			return false
		}
		if !slices.Contains(values, expectedValue) {
			matched = false
			return false
		}
		return true
	})
	return matched
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.status = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

type AccessEvent struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Host       string `json:"host"`
	StatusCode int    `json:"status_code"`
	DurationMs int64  `json:"duration_ms"`
	Route      string `json:"route"`
	Service    string `json:"service"`
	Endpoint   string `json:"endpoint"`
	UserAgent  string `json:"user_agent"`
	RemoteAddr string `json:"remote_addr"`
}

type AccessLogger struct {
	enabled bool
	logger  *slog.Logger
}

func NewAccessLogger(logger *slog.Logger, enabled bool) *AccessLogger {
	return &AccessLogger{
		enabled: enabled,
		logger:  logger,
	}
}

func (l *AccessLogger) Log(event AccessEvent) {
	if !l.enabled || l.logger == nil {
		return
	}
	l.logger.Info("access",
		slog.String("method", event.Method),
		slog.String("path", event.Path),
		slog.String("host", event.Host),
		slog.Int("status_code", event.StatusCode),
		slog.Int64("duration_ms", event.DurationMs),
		slog.String("route", event.Route),
		slog.String("service", event.Service),
		slog.String("endpoint", event.Endpoint),
		slog.String("user_agent", event.UserAgent),
		slog.String("remote_addr", event.RemoteAddr),
	)
}

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
		requests: obs.Counter(observabilityx.NewCounterSpec("vela_http_requests_total",
			observabilityx.WithDescription("Total HTTP requests handled by vela."),
			observabilityx.WithLabelKeys("route", "service", "endpoint", "status"),
		)),
		latency: obs.Histogram(observabilityx.NewHistogramSpec("vela_http_request_duration_seconds",
			observabilityx.WithDescription("HTTP request duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("route", "service"),
		)),
		reloads: obs.Counter(observabilityx.NewCounterSpec("vela_runtime_reloads_total",
			observabilityx.WithDescription("Total runtime reload attempts."),
			observabilityx.WithLabelKeys("result"),
		)),
		healthChecks: obs.Counter(observabilityx.NewCounterSpec("vela_health_checks_total",
			observabilityx.WithDescription("Total endpoint health check results."),
			observabilityx.WithLabelKeys("endpoint", "healthy"),
		)),
		routes: obs.Gauge(observabilityx.NewGaugeSpec("vela_active_routes",
			observabilityx.WithDescription("Current compiled route count."),
		)),
		services: obs.Gauge(observabilityx.NewGaugeSpec("vela_active_services",
			observabilityx.WithDescription("Current compiled service count."),
		)),
		endpoints: obs.Gauge(observabilityx.NewGaugeSpec("vela_active_endpoints",
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

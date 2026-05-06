package runtime

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"sync/atomic"
	"time"
)

type CompiledSnapshot struct {
	Entrypoints        map[string]string
	RoutesByEntrypoint map[string][]*CompiledRoute
	EntrypointMatchers map[string]*EntrypointMatcher
	Services           map[string]*ServiceRuntime
	AdminAddress       string
	AccessLogEnabled   bool
	MetricsEnabled     bool
	HealthInterval     string
	HealthTimeout      string
	ProxyEngine        string
	BuiltAt            time.Time
}

type CompiledRoute struct {
	Name       string
	Entrypoint string
	Host       string
	PathPrefix string
	Method     string
	Headers    map[string]string
	Service    *ServiceRuntime
}

type ServiceRuntime struct {
	Name          string
	Strategy      string
	Endpoints     []*EndpointRuntime
	weightedSlots []int
	rrCounter     atomic.Uint64
}

func (s *ServiceRuntime) BuildSlots() {
	s.weightedSlots = s.weightedSlots[:0]
	if s.Strategy != "weighted_round_robin" {
		return
	}
	for idx, endpoint := range s.Endpoints {
		weight := endpoint.Weight
		if weight <= 0 {
			weight = 1
		}
		for i := 0; i < weight; i++ {
			s.weightedSlots = append(s.weightedSlots, idx)
		}
	}
}

type EndpointRuntime struct {
	URL         *url.URL
	Weight      int
	Proxy       http.Handler
	Healthy     atomic.Bool
	LastChecked atomic.Int64
}

func (s *ServiceRuntime) Pick() (*EndpointRuntime, error) {
	if len(s.Endpoints) == 0 {
		return nil, errors.New("service has no endpoints")
	}
	if s.Strategy == "weighted_round_robin" && len(s.weightedSlots) > 0 {
		start := s.rrCounter.Add(1)
		for offset := range len(s.weightedSlots) {
			slot := s.weightedSlots[(int(start)+offset)%len(s.weightedSlots)]
			ep := s.Endpoints[slot]
			if ep.Healthy.Load() {
				return ep, nil
			}
		}
	} else {
		start := s.rrCounter.Add(1)
		for offset := range len(s.Endpoints) {
			ep := s.Endpoints[(int(start)+offset)%len(s.Endpoints)]
			if ep.Healthy.Load() {
				return ep, nil
			}
		}
	}
	return nil, errors.New("no healthy endpoint")
}

type Gateway struct {
	current atomic.Pointer[CompiledSnapshot]
	access  *AccessLogger
	metrics MetricsRecorder
}

func NewGateway(snapshot *CompiledSnapshot, logger *slog.Logger, accessEnabled bool, metrics MetricsRecorder) *Gateway {
	accessLogger := NewAccessLogger(logger, accessEnabled)
	if metrics == nil {
		metrics = NewNoopMetrics()
	}
	gateway := &Gateway{
		access:  accessLogger,
		metrics: metrics,
	}
	gateway.current.Store(snapshot)
	return gateway
}

func (g *Gateway) Swap(snapshot *CompiledSnapshot) {
	g.current.Store(snapshot)
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

		route := MatchRoute(snapshot.EntrypointMatchers[entrypoint], snapshot.RoutesByEntrypoint[entrypoint], r)
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
		endpoint.Proxy.ServeHTTP(recorder, r)
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

func matchHeaders(expected map[string]string, actual http.Header) bool {
	for key, expectedValue := range expected {
		values := actual.Values(key)
		if len(values) == 0 {
			return false
		}
		if !slices.Contains(values, expectedValue) {
			return false
		}
	}
	return true
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
	if logger == nil {
		logger = slog.Default()
	}
	return &AccessLogger{
		enabled: enabled,
		logger:  logger,
	}
}

func (l *AccessLogger) Log(event AccessEvent) {
	if !l.enabled {
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

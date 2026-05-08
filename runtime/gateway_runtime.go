package runtime

import (
	"log/slog"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

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
		routes := collectionlist.NewList(snapshot.RoutesByEntrypoint.Get(entrypoint)...)
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

		event := AccessEvent{
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
		}
		g.metrics.Observe(route, endpoint, recorder.status, duration)
		g.access.Log(event)
		g.logFailedRequest(event)
	})
}

func (g *Gateway) logFailedRequest(event AccessEvent) {
	if event.StatusCode < http.StatusInternalServerError || g == nil || g.access == nil || g.access.logger == nil {
		return
	}
	g.access.logger.Error("request failed",
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

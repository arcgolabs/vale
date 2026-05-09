package runtime

import (
	"log/slog"
	"net/http"
	"time"
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
		accessLogEnabled:   accessLogger.Enabled(),
		metricsEnabled:     metricsRecorderEnabled(metrics),
		middlewareRegistry: registry,
	}
	routeHandlers := newRouteHandlerIndex(snapshot, registry)
	gateway.routeHandlers.Store(routeHandlers)
	gateway.entrypointHandlers.Store(newEntrypointHandlerIndex(snapshot, routeHandlers))
	gateway.routeMatches.Store(newRouteMatchCache(snapshot))
	gateway.current.Store(snapshot)
	gateway.ObserveSnapshot(snapshot)
	return gateway
}

func (g *Gateway) Swap(snapshot *CompiledSnapshot) {
	routeHandlers := newRouteHandlerIndex(snapshot, g.middlewareRegistry)
	g.routeHandlers.Store(routeHandlers)
	g.entrypointHandlers.Store(newEntrypointHandlerIndex(snapshot, routeHandlers))
	g.routeMatches.Store(newRouteMatchCache(snapshot))
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

		if handler, ok := g.entrypointHandler(snapshot, entrypoint); ok {
			g.serveEntrypointHandler(w, r, snapshot, handler)
			return
		}

		route := g.matchRoute(snapshot, entrypoint, r)
		if route == nil {
			http.NotFound(w, r)
			return
		}

		g.serveRoute(w, r, snapshot, route)
	})
}

func (g *Gateway) entrypointHandler(snapshot *CompiledSnapshot, entrypoint string) (*entrypointHandler, bool) {
	return g.entrypointHandlers.Load().handler(snapshot, entrypoint)
}

func (g *Gateway) serveEntrypointHandler(w http.ResponseWriter, r *http.Request, snapshot *CompiledSnapshot, handler *entrypointHandler) {
	if handler == nil || !handler.matches(r) {
		http.NotFound(w, r)
		return
	}
	if handler.endpoint != nil && handler.handler != nil {
		if !handler.endpoint.Healthy.Load() {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		g.serveRouteHandler(w, r, snapshot, handler.route, handler.endpoint, handler.handler)
		return
	}
	g.serveRoute(w, r, snapshot, handler.route)
}

func (g *Gateway) serveRoute(w http.ResponseWriter, r *http.Request, snapshot *CompiledSnapshot, route *CompiledRoute) {
	if route == nil || route.Service == nil {
		http.NotFound(w, r)
		return
	}

	endpoint, err := route.Service.Pick()
	if err != nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	g.serveRouteHandler(w, r, snapshot, route, endpoint, g.routeHandler(snapshot, route, endpoint))
}

func (g *Gateway) serveRouteHandler(
	w http.ResponseWriter,
	r *http.Request,
	snapshot *CompiledSnapshot,
	route *CompiledRoute,
	endpoint *EndpointRuntime,
	handler http.Handler,
) {
	if handler == nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	accessEnabled := g.accessEnabled()
	metricsEnabled := g.metricsEnabled
	if !accessEnabled && !metricsEnabled {
		if shouldLimitRequestBody(r, snapshot.Security.MaxBodyBytes) {
			r.Body = http.MaxBytesReader(w, r.Body, snapshot.Security.MaxBodyBytes)
		}
		handler.ServeHTTP(w, r)
		return
	}

	start := time.Now()
	recorder := newStatusRecorder(w)
	if shouldLimitRequestBody(r, snapshot.Security.MaxBodyBytes) {
		r.Body = http.MaxBytesReader(recorder, r.Body, snapshot.Security.MaxBodyBytes)
	}
	handler.ServeHTTP(recorder, r)
	duration := time.Since(start)
	g.observeRequest(r, route, endpoint, recorder.status, duration, accessEnabled, metricsEnabled)
}

func (g *Gateway) observeRequest(
	r *http.Request,
	route *CompiledRoute,
	endpoint *EndpointRuntime,
	status int,
	duration time.Duration,
	accessEnabled bool,
	metricsEnabled bool,
) {
	if metricsEnabled {
		g.metrics.Observe(route, endpoint, status, duration)
	}
	if !accessEnabled {
		return
	}

	event := AccessEvent{
		Method:     r.Method,
		Path:       r.URL.Path,
		Host:       r.Host,
		StatusCode: status,
		DurationMs: duration.Milliseconds(),
		Route:      route.Name,
		Service:    route.Service.Name,
		Endpoint:   endpoint.URL.String(),
		UserAgent:  r.UserAgent(),
		RemoteAddr: r.RemoteAddr,
	}
	if accessEnabled {
		g.access.Log(event)
	}
	if g.failureLogEnabled(r) {
		g.logFailedRequest(event)
	}
}

func (g *Gateway) accessEnabled() bool {
	return g != nil && g.accessLogEnabled
}

func (g *Gateway) failureLogEnabled(r *http.Request) bool {
	if g == nil || g.access == nil || g.access.logger == nil || r == nil {
		return false
	}
	return g.access.logger.Enabled(r.Context(), slog.LevelError)
}

type metricsState interface {
	Enabled() bool
}

func metricsRecorderEnabled(recorder MetricsRecorder) bool {
	if recorder == nil {
		return false
	}
	state, ok := recorder.(metricsState)
	if !ok {
		return true
	}
	return state.Enabled()
}

func shouldLimitRequestBody(r *http.Request, maxBodyBytes int64) bool {
	return maxBodyBytes > 0 && r != nil && r.Body != nil && r.ContentLength != 0
}

func (g *Gateway) matchRoute(snapshot *CompiledSnapshot, entrypoint string, request *http.Request) *CompiledRoute {
	cache := g.routeMatches.Load()
	if route, ok := cache.get(snapshot, entrypoint, request); ok {
		g.ObserveRouteCache(true)
		return route
	}
	route := matchSnapshotRoute(snapshot, entrypoint, request)
	cache.add(snapshot, entrypoint, request, route)
	g.ObserveRouteCache(false)
	return route
}

func (g *Gateway) routeHandler(snapshot *CompiledSnapshot, route *CompiledRoute, endpoint *EndpointRuntime) http.Handler {
	if handler, ok := g.routeHandlers.Load().handler(snapshot, route, endpoint); ok {
		return handler
	}
	if endpoint == nil || endpoint.Proxy == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		})
	}
	return WrapMiddlewaresWithRegistry(endpoint.Proxy, route.Middlewares, g.middlewareRegistry)
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

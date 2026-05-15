package runtime_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func BenchmarkGatewayHandlerByRouteCount(b *testing.B) {
	for _, routeCount := range []int{1, 100, 1000} {
		b.Run(benchmarkRouteCountName(routeCount), func(b *testing.B) {
			snapshot := benchmarkGatewaySnapshot(b, routeCount)
			logger := slog.New(slog.DiscardHandler)
			handler := valeruntime.NewGateway(snapshot, logger, false, valeruntime.NewNoopMetrics()).Handler("web")
			target := benchmarkRouteURL(routeCount - 1)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, target, http.NoBody)
				req.Host = "api.example.com"
				recorder := httptest.NewRecorder()

				handler.ServeHTTP(recorder, req)
				if recorder.Code != http.StatusNoContent {
					b.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
				}
			}
		})
	}
}

func BenchmarkGatewayHandlerRouteCacheMiss(b *testing.B) {
	const (
		routeCount   = 1000
		requestCount = 8192
		requestMask  = requestCount - 1
	)

	snapshot := benchmarkGatewaySnapshot(b, routeCount)
	logger := slog.New(slog.DiscardHandler)
	handler := valeruntime.NewGateway(snapshot, logger, false, valeruntime.NewNoopMetrics()).Handler("web")
	requests := benchmarkGatewayRequests(b, benchmarkRouteURL(routeCount-1), requestCount)
	writer := newBenchmarkResponseWriter()

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		writer.Reset()
		handler.ServeHTTP(writer, requests[i&requestMask])
		if writer.status != http.StatusNoContent {
			b.Fatalf("status = %d, want %d", writer.status, http.StatusNoContent)
		}
	}
}

func benchmarkGatewaySnapshot(b *testing.B, routeCount int) *valeruntime.CompiledSnapshot {
	b.Helper()

	endpoint, err := valeruntime.NewEndpoint(
		"http://127.0.0.1:8081",
		1,
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	if err != nil {
		b.Fatal(err)
	}
	service := valeruntime.NewService("api", "round_robin", endpoint)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":0", valeruntime.EntrypointRuntime{Name: "web", Address: ":0"}).
		AddService(service)

	for i := range routeCount {
		snapshot.AddRoute(
			valeruntime.NewRoute(benchmarkRouteName(i), "web", service).
				WithHost("api.example.com").
				WithPathPrefix(benchmarkRoutePath(i)).
				WithMethod(http.MethodGet),
		)
	}
	return snapshot.BuildMatchers()
}

func benchmarkGatewayRequests(b *testing.B, target string, count int) []*http.Request {
	b.Helper()

	requests := make([]*http.Request, count)
	for i := range count {
		req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, target+"/"+strconv.Itoa(i), http.NoBody)
		req.Host = "api.example.com"
		requests[i] = req
	}
	return requests
}

type benchmarkResponseWriter struct {
	header http.Header
	status int
}

func newBenchmarkResponseWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{header: make(http.Header)}
}

func (w *benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *benchmarkResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(body), nil
}

func (w *benchmarkResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *benchmarkResponseWriter) Reset() {
	for key := range w.header {
		delete(w.header, key)
	}
	w.status = 0
}

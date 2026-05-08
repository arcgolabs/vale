package runtime_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
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

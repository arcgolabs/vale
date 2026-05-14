package runtime_test

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func BenchmarkCompiledSnapshotQueryRoutesByHostAndPathPrefix(b *testing.B) {
	snapshot := benchmarkCatalogSnapshot()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		routes := snapshot.QueryRoutes(valeruntime.RouteFilter{
			Host:       "api.example.com",
			PathPrefix: "/api/5",
		})
		if routes.Len() != 10 {
			b.Fatalf("routes len = %d, want 10", routes.Len())
		}
	}
}

func BenchmarkCompiledSnapshotQueryRoutesByEntrypointServiceHostPath(b *testing.B) {
	snapshot := benchmarkCatalogSnapshot()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		routes := snapshot.QueryRoutes(valeruntime.RouteFilter{
			Entrypoint: "web",
			Host:       "api.example.com",
			PathPrefix: "/api/5",
			Service:    "api",
		})
		if routes.Len() != 10 {
			b.Fatalf("routes len = %d, want 10", routes.Len())
		}
	}
}

func benchmarkCatalogSnapshot() *valeruntime.CompiledSnapshot {
	endpointURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		panic(err)
	}
	endpoint := &valeruntime.EndpointRuntime{URL: endpointURL}
	service := valeruntime.NewService("api", "round_robin", endpoint)

	snapshot := valeruntime.NewSnapshot().AddEntrypoint("web", ":8080", valeruntime.EntrypointRuntime{})
	for idx := range 100 {
		route := valeruntime.NewRoute("route-"+strconv.Itoa(idx), "web", service).
			WithHost("api.example.com").
			WithPathPrefix("/api/" + strconv.Itoa(idx%10)).
			WithMethod(http.MethodGet)
		snapshot = snapshot.AddRoute(route)
	}
	return snapshot.
		AddService(service).
		BuildMatchers()
}

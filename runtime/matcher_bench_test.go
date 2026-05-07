package runtime_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/bitset"
	"github.com/arcgolabs/collectionx/mapping"
	velaruntime "github.com/arcgolabs/vela/runtime"
)

func BenchmarkMatchRouteByRouteCount(b *testing.B) {
	for _, routeCount := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			routes := benchmarkRoutes(routeCount)
			matcher := velaruntime.BuildEntrypointMatcher(routes)
			target := fmt.Sprintf("http://api.example.com/api/%04d/users", routeCount-1)
			req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, target, http.NoBody)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				route := velaruntime.MatchRoute(matcher, routes, req)
				if route == nil {
					b.Fatal("route did not match")
				}
			}
		})
	}
}

func benchmarkRoutes(count int) []*velaruntime.CompiledRoute {
	routes := make([]*velaruntime.CompiledRoute, 0, count)
	for i := range count {
		route := &velaruntime.CompiledRoute{
			Name:       fmt.Sprintf("api-%04d", i),
			Entrypoint: "web",
			Host:       "api.example.com",
			PathPrefix: fmt.Sprintf("/api/%04d", i),
			Method:     http.MethodGet,
			Headers:    mapping.NewMap[string, string](),
			Predicates: bitset.New(),
		}
		route.Predicates.Set(velaruntime.PredicateHost)
		route.Predicates.Set(velaruntime.PredicatePathPrefix)
		route.Predicates.Set(velaruntime.PredicateMethod)
		routes = append(routes, route)
	}
	return routes
}

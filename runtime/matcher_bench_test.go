package runtime

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/bitset"
	"github.com/arcgolabs/collectionx/mapping"
)

func BenchmarkMatchRouteByRouteCount(b *testing.B) {
	for _, routeCount := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), func(b *testing.B) {
			routes := benchmarkRoutes(routeCount)
			matcher := BuildEntrypointMatcher(routes)
			target := fmt.Sprintf("http://api.example.com/api/%04d/users", routeCount-1)
			req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, target, http.NoBody)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				route := MatchRoute(matcher, routes, req)
				if route == nil {
					b.Fatal("route did not match")
				}
			}
		})
	}
}

func benchmarkRoutes(count int) []*CompiledRoute {
	routes := make([]*CompiledRoute, 0, count)
	for i := range count {
		route := &CompiledRoute{
			Name:       fmt.Sprintf("api-%04d", i),
			Entrypoint: "web",
			Host:       "api.example.com",
			PathPrefix: fmt.Sprintf("/api/%04d", i),
			Method:     http.MethodGet,
			Headers:    mapping.NewMap[string, string](),
			Predicates: bitset.New(),
		}
		route.Predicates.Set(PredicateHost)
		route.Predicates.Set(PredicatePathPrefix)
		route.Predicates.Set(PredicateMethod)
		routes = append(routes, route)
	}
	return routes
}

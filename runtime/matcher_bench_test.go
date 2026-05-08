package runtime_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func BenchmarkMatchRouteByRouteCount(b *testing.B) {
	for _, routeCount := range []int{100, 1000, 10000} {
		b.Run(benchmarkRouteCountName(routeCount), func(b *testing.B) {
			routes := benchmarkRoutes(routeCount)
			matcher := valeruntime.BuildEntrypointMatcher(routes)
			target := benchmarkRouteURL(routeCount - 1)
			req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, target, http.NoBody)

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				route := valeruntime.MatchRoute(matcher, routes, req)
				if route == nil {
					b.Fatal("route did not match")
				}
			}
		})
	}
}

func benchmarkRoutes(count int) *collectionlist.List[*valeruntime.CompiledRoute] {
	routes := collectionlist.NewListWithCapacity[*valeruntime.CompiledRoute](count)
	for i := range count {
		route := &valeruntime.CompiledRoute{
			Name:       benchmarkRouteName(i),
			Entrypoint: "web",
			Host:       "api.example.com",
			PathPrefix: benchmarkRoutePath(i),
			Method:     http.MethodGet,
			Headers:    mapping.NewMap[string, string](),
			Predicates: bitset.New(),
		}
		route.Predicates.Set(valeruntime.PredicateHost)
		route.Predicates.Set(valeruntime.PredicatePathPrefix)
		route.Predicates.Set(valeruntime.PredicateMethod)
		routes.Add(route)
	}
	return routes
}

func benchmarkRouteCountName(routeCount int) string {
	return fmt.Sprintf("routes_%d", routeCount)
}

func benchmarkRouteName(index int) string {
	return fmt.Sprintf("api-%04d", index)
}

func benchmarkRoutePath(index int) string {
	return fmt.Sprintf("/api/%04d", index)
}

func benchmarkRouteURL(index int) string {
	return fmt.Sprintf("http://api.example.com%s/users", benchmarkRoutePath(index))
}

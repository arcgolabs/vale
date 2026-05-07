package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/bitset"
	velaruntime "github.com/arcgolabs/vela/runtime"
)

func TestMatchRoutePrioritizesHostAndPredicates(t *testing.T) {
	t.Parallel()

	routes := []*velaruntime.CompiledRoute{
		{
			Name:       "fallback",
			PathPrefix: "/",
		},
		{
			Name:       "host-short",
			Host:       "api.example.com",
			PathPrefix: "/api",
		},
		{
			Name:       "host-long-method",
			Host:       "api.example.com",
			PathPrefix: "/api/v1",
			Method:     http.MethodPost,
		},
		{
			Name:       "wildcard",
			Host:       "*.example.com",
			PathPrefix: "/api/v1",
		},
	}
	matcher := velaruntime.BuildEntrypointMatcher(routes)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://api.example.com/api/v1/users", http.NoBody)
	got := velaruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "host-long-method" {
		t.Fatalf("matched route = %v, want host-long-method", routeName(got))
	}

	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://shop.example.com/api/v1/users", http.NoBody)
	got = velaruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "wildcard" {
		t.Fatalf("matched route = %v, want wildcard", routeName(got))
	}

	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://other.test/anything", http.NoBody)
	got = velaruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "fallback" {
		t.Fatalf("matched route = %v, want fallback", routeName(got))
	}
}

func TestMatchRouteStripsPortFromHost(t *testing.T) {
	t.Parallel()

	routes := []*velaruntime.CompiledRoute{
		{
			Name: "api",
			Host: "api.example.com",
		},
	}
	matcher := velaruntime.BuildEntrypointMatcher(routes)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com:8080/", http.NoBody)

	got := velaruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "api" {
		t.Fatalf("matched route = %v, want api", routeName(got))
	}
}

func TestMatchRouteFallsBackToShorterPrefixWhenLongerPredicateMisses(t *testing.T) {
	t.Parallel()

	routes := []*velaruntime.CompiledRoute{
		routeWithPredicates(&velaruntime.CompiledRoute{
			Name:       "api-short",
			Host:       "api.example.com",
			PathPrefix: "/api",
		}),
		routeWithPredicates(&velaruntime.CompiledRoute{
			Name:       "api-v1-post",
			Host:       "api.example.com",
			PathPrefix: "/api/v1",
			Method:     http.MethodPost,
		}),
	}
	matcher := velaruntime.BuildEntrypointMatcher(routes)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com/api/v1/users", http.NoBody)

	got := velaruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "api-short" {
		t.Fatalf("matched route = %v, want api-short", routeName(got))
	}
}

func routeName(route *velaruntime.CompiledRoute) string {
	if route == nil {
		return "<nil>"
	}
	return route.Name
}

func routeWithPredicates(route *velaruntime.CompiledRoute) *velaruntime.CompiledRoute {
	route.Predicates = bitset.New()
	if route.Host != "" {
		route.Predicates.Set(velaruntime.PredicateHost)
	}
	if route.PathPrefix != "" {
		route.Predicates.Set(velaruntime.PredicatePathPrefix)
	}
	if route.Method != "" {
		route.Predicates.Set(velaruntime.PredicateMethod)
	}
	if route.Headers.Len() > 0 {
		route.Predicates.Set(velaruntime.PredicateHeaders)
	}
	return route
}

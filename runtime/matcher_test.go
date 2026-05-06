package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/bitset"
)

func TestMatchRoutePrioritizesHostAndPredicates(t *testing.T) {
	t.Parallel()

	routes := []*CompiledRoute{
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
	matcher := BuildEntrypointMatcher(routes)

	req := httptest.NewRequest(http.MethodPost, "http://api.example.com/api/v1/users", nil)
	got := MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "host-long-method" {
		t.Fatalf("matched route = %v, want host-long-method", routeName(got))
	}

	req = httptest.NewRequest(http.MethodGet, "http://shop.example.com/api/v1/users", nil)
	got = MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "wildcard" {
		t.Fatalf("matched route = %v, want wildcard", routeName(got))
	}

	req = httptest.NewRequest(http.MethodGet, "http://other.test/anything", nil)
	got = MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "fallback" {
		t.Fatalf("matched route = %v, want fallback", routeName(got))
	}
}

func TestMatchRouteStripsPortFromHost(t *testing.T) {
	t.Parallel()

	routes := []*CompiledRoute{
		{
			Name: "api",
			Host: "api.example.com",
		},
	}
	matcher := BuildEntrypointMatcher(routes)
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com:8080/", nil)

	got := MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "api" {
		t.Fatalf("matched route = %v, want api", routeName(got))
	}
}

func TestMatchRouteFallsBackToShorterPrefixWhenLongerPredicateMisses(t *testing.T) {
	t.Parallel()

	routes := []*CompiledRoute{
		routeWithPredicates(&CompiledRoute{
			Name:       "api-short",
			Host:       "api.example.com",
			PathPrefix: "/api",
		}),
		routeWithPredicates(&CompiledRoute{
			Name:       "api-v1-post",
			Host:       "api.example.com",
			PathPrefix: "/api/v1",
			Method:     http.MethodPost,
		}),
	}
	matcher := BuildEntrypointMatcher(routes)
	req := httptest.NewRequest(http.MethodGet, "http://api.example.com/api/v1/users", nil)

	got := MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "api-short" {
		t.Fatalf("matched route = %v, want api-short", routeName(got))
	}
}

func routeName(route *CompiledRoute) string {
	if route == nil {
		return "<nil>"
	}
	return route.Name
}

func routeWithPredicates(route *CompiledRoute) *CompiledRoute {
	route.Predicates = bitset.New()
	if route.Host != "" {
		route.Predicates.Set(PredicateHost)
	}
	if route.PathPrefix != "" {
		route.Predicates.Set(PredicatePathPrefix)
	}
	if route.Method != "" {
		route.Predicates.Set(PredicateMethod)
	}
	if route.Headers.Len() > 0 {
		route.Predicates.Set(PredicateHeaders)
	}
	return route
}

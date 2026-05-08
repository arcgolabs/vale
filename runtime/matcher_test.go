package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestMatchRoutePrioritizesHostAndPredicates(t *testing.T) {
	t.Parallel()

	routes := collectionlist.NewList(
		&valeruntime.CompiledRoute{
			Name:       "fallback",
			PathPrefix: "/",
		},
		&valeruntime.CompiledRoute{
			Name:       "host-short",
			Host:       "api.example.com",
			PathPrefix: "/api",
		},
		&valeruntime.CompiledRoute{
			Name:       "host-long-method",
			Host:       "api.example.com",
			PathPrefix: "/api/v1",
			Method:     http.MethodPost,
		},
		&valeruntime.CompiledRoute{
			Name:       "wildcard",
			Host:       "*.example.com",
			PathPrefix: "/api/v1",
		},
	)
	matcher := valeruntime.BuildEntrypointMatcher(routes)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://api.example.com/api/v1/users", http.NoBody)
	got := valeruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "host-long-method" {
		t.Fatalf("matched route = %v, want host-long-method", routeName(got))
	}

	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://shop.example.com/api/v1/users", http.NoBody)
	got = valeruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "wildcard" {
		t.Fatalf("matched route = %v, want wildcard", routeName(got))
	}

	req = httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://other.test/anything", http.NoBody)
	got = valeruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "fallback" {
		t.Fatalf("matched route = %v, want fallback", routeName(got))
	}
}

func TestMatchRouteStripsPortFromHost(t *testing.T) {
	t.Parallel()

	routes := collectionlist.NewList(
		&valeruntime.CompiledRoute{
			Name: "api",
			Host: "api.example.com",
		},
	)
	matcher := valeruntime.BuildEntrypointMatcher(routes)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com:8080/", http.NoBody)

	got := valeruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "api" {
		t.Fatalf("matched route = %v, want api", routeName(got))
	}
}

func TestMatchRouteFallsBackToShorterPrefixWhenLongerPredicateMisses(t *testing.T) {
	t.Parallel()

	routes := collectionlist.NewList(
		routeWithPredicates(&valeruntime.CompiledRoute{
			Name:       "api-short",
			Host:       "api.example.com",
			PathPrefix: "/api",
		}),
		routeWithPredicates(&valeruntime.CompiledRoute{
			Name:       "api-v1-post",
			Host:       "api.example.com",
			PathPrefix: "/api/v1",
			Method:     http.MethodPost,
		}),
	)
	matcher := valeruntime.BuildEntrypointMatcher(routes)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.example.com/api/v1/users", http.NoBody)

	got := valeruntime.MatchRoute(matcher, routes, req)
	if got == nil || got.Name != "api-short" {
		t.Fatalf("matched route = %v, want api-short", routeName(got))
	}
}

func routeName(route *valeruntime.CompiledRoute) string {
	if route == nil {
		return "<nil>"
	}
	return route.Name
}

func routeWithPredicates(route *valeruntime.CompiledRoute) *valeruntime.CompiledRoute {
	route.Predicates = bitset.New()
	if route.Host != "" {
		route.Predicates.Set(valeruntime.PredicateHost)
	}
	if route.PathPrefix != "" {
		route.Predicates.Set(valeruntime.PredicatePathPrefix)
	}
	if route.Method != "" {
		route.Predicates.Set(valeruntime.PredicateMethod)
	}
	if route.Headers.Len() > 0 {
		route.Predicates.Set(valeruntime.PredicateHeaders)
	}
	return route
}

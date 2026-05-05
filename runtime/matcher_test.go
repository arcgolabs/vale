package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

func routeName(route *CompiledRoute) string {
	if route == nil {
		return "<nil>"
	}
	return route.Name
}

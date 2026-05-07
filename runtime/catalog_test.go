package runtime

import (
	"net/http"
	"testing"
)

func TestCatalogQueriesRoutesByServiceAndEntrypoint(t *testing.T) {
	t.Parallel()

	apiEndpoint, err := NewEndpoint("http://127.0.0.1:8081", 1, http.DefaultServeMux)
	if err != nil {
		t.Fatal(err)
	}
	webEndpoint, err := NewEndpoint("http://127.0.0.1:8082", 1, http.DefaultServeMux)
	if err != nil {
		t.Fatal(err)
	}
	api := NewService("api", "round_robin", apiEndpoint)
	web := NewService("web", "round_robin", webEndpoint)
	snapshot := NewSnapshot().
		AddEntrypoint("web", ":8080", EntrypointRuntime{}).
		AddService(api).
		AddService(web).
		AddRoute(NewRoute("api-route", "web", api).WithHost("api.example.com").WithPathPrefix("/api")).
		AddRoute(NewRoute("web-route", "web", web).WithPathPrefix("/")).
		BuildMatchers()

	byService := snapshot.QueryRoutes(RouteFilter{Service: "api"})
	if byService.Len() != 1 {
		t.Fatalf("routes by service len = %d, want 1", byService.Len())
	}
	route, _ := byService.Get(0)
	if route.Name != "api-route" {
		t.Fatalf("route by service = %q, want api-route", route.Name)
	}

	byEntrypoint := snapshot.QueryRoutes(RouteFilter{Entrypoint: "web"})
	if byEntrypoint.Len() != 2 {
		t.Fatalf("routes by entrypoint len = %d, want 2", byEntrypoint.Len())
	}
}

func TestCatalogFallsBackWhenMissing(t *testing.T) {
	t.Parallel()

	endpoint, err := NewEndpoint("http://127.0.0.1:8081", 1, http.DefaultServeMux)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService("api", "round_robin", endpoint)
	snapshot := NewSnapshot().
		AddEntrypoint("web", ":8080", EntrypointRuntime{}).
		AddService(service).
		AddRoute(NewRoute("api-route", "web", service).WithPathPrefix("/api")).
		BuildMatchers()
	snapshot.Catalog = nil

	routes := snapshot.QueryRoutes(RouteFilter{PathPrefix: "/api"})
	if routes.Len() != 1 {
		t.Fatalf("fallback routes len = %d, want 1", routes.Len())
	}
}

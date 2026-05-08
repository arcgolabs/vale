package runtime_test

import (
	"net/http"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestCatalogQueriesRoutesByServiceAndEntrypoint(t *testing.T) {
	t.Parallel()

	apiEndpoint, err := valeruntime.NewEndpoint("http://127.0.0.1:8081", 1, http.DefaultServeMux)
	if err != nil {
		t.Fatal(err)
	}
	webEndpoint, err := valeruntime.NewEndpoint("http://127.0.0.1:8082", 1, http.DefaultServeMux)
	if err != nil {
		t.Fatal(err)
	}
	api := valeruntime.NewService("api", "round_robin", apiEndpoint)
	web := valeruntime.NewService("web", "round_robin", webEndpoint)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":8080", valeruntime.EntrypointRuntime{}).
		AddService(api).
		AddService(web).
		AddRoute(valeruntime.NewRoute("api-route", "web", api).WithHost("api.example.com").WithPathPrefix("/api")).
		AddRoute(valeruntime.NewRoute("web-route", "web", web).WithPathPrefix("/")).
		BuildMatchers()

	byService := snapshot.QueryRoutes(valeruntime.RouteFilter{Service: "api"})
	if byService.Len() != 1 {
		t.Fatalf("routes by service len = %d, want 1", byService.Len())
	}
	route, _ := byService.Get(0)
	if route.Name != "api-route" {
		t.Fatalf("route by service = %q, want api-route", route.Name)
	}

	byEntrypoint := snapshot.QueryRoutes(valeruntime.RouteFilter{Entrypoint: "web"})
	if byEntrypoint.Len() != 2 {
		t.Fatalf("routes by entrypoint len = %d, want 2", byEntrypoint.Len())
	}
}

func TestCatalogFallsBackWhenMissing(t *testing.T) {
	t.Parallel()

	endpoint, err := valeruntime.NewEndpoint("http://127.0.0.1:8081", 1, http.DefaultServeMux)
	if err != nil {
		t.Fatal(err)
	}
	service := valeruntime.NewService("api", "round_robin", endpoint)
	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":8080", valeruntime.EntrypointRuntime{}).
		AddService(service).
		AddRoute(valeruntime.NewRoute("api-route", "web", service).WithPathPrefix("/api")).
		BuildMatchers()
	snapshot.Catalog = nil

	routes := snapshot.QueryRoutes(valeruntime.RouteFilter{PathPrefix: "/api"})
	if routes.Len() != 1 {
		t.Fatalf("fallback routes len = %d, want 1", routes.Len())
	}
}

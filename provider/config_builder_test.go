package provider_test

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
)

func TestAppendSortedServices(t *testing.T) {
	cfg := provider.NewEntrypointConfig("web", ":8080")
	services := mapping.NewMap[string, *config.Service]()
	services.Set("b", &config.Service{Name: "b"})
	services.Set("a", &config.Service{Name: "a"})

	provider.AppendSortedServices(cfg, services)

	got := []string{cfg.Services[0].Name, cfg.Services[1].Name}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("services = %v", got)
	}
}

func TestAppendSortedRoutes(t *testing.T) {
	cfg := provider.NewEntrypointConfig("web", ":8080")
	routes := mapping.NewMap[string, config.Route]()
	routes.Set("b", config.Route{Name: "b"})
	routes.Set("a", config.Route{Name: "a"})

	provider.AppendSortedRoutes(cfg, routes)

	got := []string{cfg.Routes[0].Name, cfg.Routes[1].Name}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("routes = %v", got)
	}
}

func TestConfigBuilderFluentAPI(t *testing.T) {
	t.Parallel()

	cfg, err := provider.NewConfigBuilder().
		Entrypoint("websecure", ":8443",
			provider.EntrypointTLS("cert.pem", "key.pem"),
			provider.EntrypointACME("ops@example.com", "", "example.com"),
		).
		ServiceWithStrategy("api", "weighted_round_robin",
			provider.ConfigEndpoint("http://127.0.0.1:8081", 2),
			provider.ConfigEndpoint("http://127.0.0.1:8082", 1),
		).
		MiddlewareNamed("strip-api",
			provider.MiddlewareStripPrefix("/api"),
			provider.MiddlewareRequestHeader("X-Test", "ok"),
			provider.MiddlewareResponseHeader("X-Response", "set"),
			provider.MiddlewareMaxBodyBytes(1024),
		).
		RouteTo("api", "websecure", "api",
			provider.RouteHost("api.example.com"),
			provider.RoutePathPrefix("/api"),
			provider.RouteMethod(http.MethodGet),
			provider.RouteHeader("X-Env", "test"),
			provider.RouteMiddlewares("strip-api"),
		).
		Admin(":19090").
		Observability(true, true).
		Health("5s", "2s").
		BuildValidated()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Entrypoints[0].TLS == nil || cfg.Entrypoints[0].ACME == nil {
		t.Fatalf("entrypoint tls/acme not set: %#v", cfg.Entrypoints[0])
	}
	if cfg.Services[0].Strategy != "weighted_round_robin" || len(cfg.Services[0].Endpoints) != 2 {
		t.Fatalf("service = %#v", cfg.Services[0])
	}
	if cfg.Middlewares[0].StripPrefix != "/api" || cfg.Middlewares[0].RequestHeaders["X-Test"] != "ok" {
		t.Fatalf("middleware = %#v", cfg.Middlewares[0])
	}
	if cfg.Routes[0].Method != http.MethodGet || cfg.Routes[0].Middlewares[0] != "strip-api" {
		t.Fatalf("route = %#v", cfg.Routes[0])
	}
}

func TestConfigBuilderBuildValidatedReturnsAccumulatedErrors(t *testing.T) {
	t.Parallel()

	_, err := provider.NewConfigBuilder().
		Entrypoint("", "").
		ServiceWithStrategy("api", "random", provider.ConfigEndpoint("not-a-url", 1)).
		RouteTo("api", "web", "api").
		BuildValidated()
	if err == nil {
		t.Fatal("BuildValidated returned nil error")
	}
	message := err.Error()
	for _, want := range []string{
		"entrypoint name cannot be empty",
		"endpoint \"not-a-url\" is invalid",
		"unsupported strategy",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("BuildValidated error = %q, want %q", message, want)
		}
	}
}

package vale_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/config"
)

func TestNewUsesDefaultConfigWhenNoSourceIsConfigured(t *testing.T) {
	t.Parallel()

	gateway, err := vale.New(vale.WithLogger(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
	}
}

func TestNewDoesNotApplyBuilderRegistryUnlessConfigured(t *testing.T) {
	t.Parallel()

	sawNilMiddleware := false
	_, err := vale.New(
		vale.WithLogger(slog.New(slog.DiscardHandler)),
		func(cfg *vale.Config) error {
			sawNilMiddleware = cfg.Middleware == nil
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !sawNilMiddleware {
		t.Fatal("custom option observed an implicit middleware registry")
	}
}

func TestDefaultConfigModelIsValid(t *testing.T) {
	t.Parallel()

	if err := config.Validate(config.Default()); err != nil {
		t.Fatal(err)
	}
}

func TestRootPackageRuntimeBuilders(t *testing.T) {
	t.Parallel()

	endpoint, err := vale.NewEndpoint("http://127.0.0.1:8081", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	service := vale.NewService("api", "round_robin", endpoint)
	route := vale.NewRoute("api", "web", service).WithPathPrefix("/api")
	snapshot := vale.NewSnapshot().
		AddEntrypoint("web", ":8080", vale.RuntimeEntrypoint{}).
		AddService(service).
		AddRoute(route).
		BuildMatchers()

	if snapshot.Routes().Len() != 1 {
		t.Fatalf("routes = %d, want 1", snapshot.Routes().Len())
	}
}

func TestRootPackageConfigBuilder(t *testing.T) {
	t.Parallel()

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		Service("api", "http://127.0.0.1:8081").
		RouteTo("api", "web", "api", vale.RoutePathPrefix("/api")).
		Admin(":19090").
		Build()

	if err := config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

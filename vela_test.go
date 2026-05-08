package vela_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/config"
)

func TestNewUsesDefaultConfigWhenNoSourceIsConfigured(t *testing.T) {
	t.Parallel()

	gateway, err := vela.New(vela.WithLogger(slog.New(slog.DiscardHandler)))
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
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

	endpoint, err := vela.NewEndpoint("http://127.0.0.1:8081", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	service := vela.NewService("api", "round_robin", endpoint)
	route := vela.NewRoute("api", "web", service).WithPathPrefix("/api")
	snapshot := vela.NewSnapshot().
		AddEntrypoint("web", ":8080", vela.RuntimeEntrypoint{}).
		AddService(service).
		AddRoute(route).
		BuildMatchers()

	if snapshot.Routes().Len() != 1 {
		t.Fatalf("routes = %d, want 1", snapshot.Routes().Len())
	}
}

func TestRootPackageConfigBuilder(t *testing.T) {
	t.Parallel()

	cfg := vela.NewConfigBuilder().
		Entrypoint("web", ":8080").
		Service("api", "http://127.0.0.1:8081").
		RouteTo("api", "web", "api", vela.RoutePathPrefix("/api")).
		Admin(":19090").
		Build()

	if err := config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

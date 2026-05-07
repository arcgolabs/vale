package vela

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/arcgolabs/vela/config"
)

func TestNewUsesDefaultConfigWhenNoSourceIsConfigured(t *testing.T) {
	t.Parallel()

	gateway, err := New(WithLogger(slog.New(slog.DiscardHandler)))
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

	endpoint, err := NewEndpoint("http://127.0.0.1:8081", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService("api", "round_robin", endpoint)
	route := NewRoute("api", "web", service).WithPathPrefix("/api")
	snapshot := NewSnapshot().
		AddEntrypoint("web", ":8080", RuntimeEntrypoint{}).
		AddService(service).
		AddRoute(route).
		BuildMatchers()

	if snapshot.Routes().Len() != 1 {
		t.Fatalf("routes = %d, want 1", snapshot.Routes().Len())
	}
}

func TestRootPackageConfigBuilder(t *testing.T) {
	t.Parallel()

	cfg := NewConfigBuilder().
		Entrypoint("web", ":8080").
		Service("api", "http://127.0.0.1:8081").
		RouteTo("api", "web", "api", RoutePathPrefix("/api")).
		Admin(":19090").
		Build()

	if err := config.Validate(cfg); err != nil {
		t.Fatal(err)
	}
}

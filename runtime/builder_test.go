package runtime_test

import (
	"net/http"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestSnapshotBuilderBuildsMatcher(t *testing.T) {
	t.Parallel()

	endpoint, err := valeruntime.NewEndpoint("http://127.0.0.1:8081", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	service := valeruntime.NewService("api", "round_robin", endpoint)
	route := valeruntime.NewRoute("api", "web", service).
		WithHost("API.EXAMPLE.COM").
		WithPathPrefix("/api").
		WithMethod(http.MethodGet)

	snapshot := valeruntime.NewSnapshot().
		AddEntrypoint("web", ":8080", valeruntime.EntrypointRuntime{}).
		AddService(service).
		AddRoute(route).
		BuildMatchers()

	if snapshot.Entrypoints.Len() != 1 || snapshot.Services.Len() != 1 || snapshot.Routes().Len() != 1 {
		t.Fatalf("snapshot counts = entrypoints %d services %d routes %d", snapshot.Entrypoints.Len(), snapshot.Services.Len(), snapshot.Routes().Len())
	}
	if matcher, ok := snapshot.EntrypointMatchers.Get("web"); !ok || matcher == nil {
		t.Fatal("matcher was not built")
	}
}

func TestNewEndpointRejectsRelativeURL(t *testing.T) {
	t.Parallel()

	_, err := valeruntime.NewEndpoint("/api", 1, http.NotFoundHandler())
	if err == nil {
		t.Fatal("NewEndpoint returned nil error for relative URL")
	}
}

func TestHandlerEndpointIsSelectableWithoutURL(t *testing.T) {
	t.Parallel()

	endpoint, err := valeruntime.NewHandlerEndpoint("static", 0, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Kind != valeruntime.EndpointKindHandler {
		t.Fatalf("endpoint kind = %q, want %q", endpoint.Kind, valeruntime.EndpointKindHandler)
	}
	if endpoint.Name != "static" {
		t.Fatalf("endpoint name = %q, want static", endpoint.Name)
	}
	if identifier := endpoint.Identifier(); identifier != "handler:static" {
		t.Fatalf("endpoint identifier = %q, want handler:static", identifier)
	}
	if endpoint.URL != nil {
		t.Fatalf("handler endpoint URL = %v, want nil", endpoint.URL)
	}
	if endpoint.Weight != 1 {
		t.Fatalf("handler endpoint weight = %d, want 1", endpoint.Weight)
	}

	endpoint.Healthy.Store(false)
	if !endpoint.Selectable() {
		t.Fatal("handler endpoint should remain selectable independently of HTTP health")
	}
}

func TestNewHandlerEndpointRejectsBlankName(t *testing.T) {
	t.Parallel()

	_, err := valeruntime.NewHandlerEndpoint(" ", 1, http.NotFoundHandler())
	if err == nil {
		t.Fatal("NewHandlerEndpoint returned nil error for blank name")
	}
}

func TestNewHandlerEndpointRejectsTypedNilHandler(t *testing.T) {
	t.Parallel()

	var handler http.HandlerFunc
	if _, err := valeruntime.NewHandlerEndpoint("static", 1, handler); err == nil {
		t.Fatal("NewHandlerEndpoint returned nil error for typed-nil handler")
	}
}

func TestHandlerEndpointServiceViewUsesKindAndNameWithoutURL(t *testing.T) {
	t.Parallel()

	endpoint, err := valeruntime.NewHandlerEndpoint("static", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	views := valeruntime.NewSnapshot().
		AddService(valeruntime.NewService("static", "round_robin", endpoint)).
		ServicesView()
	service, ok := views.GetFirst()
	if !ok {
		t.Fatal("ServicesView returned no service")
	}
	view, ok := service.Endpoints.GetFirst()
	if !ok {
		t.Fatal("ServicesView returned no endpoint")
	}
	if view.Kind != valeruntime.EndpointKindHandler || view.Name != "static" || view.URL != "" {
		t.Fatalf("handler endpoint view = %#v, want kind handler, name static, and empty URL", view)
	}
}

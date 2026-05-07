package runtime_test

import (
	"net/http"
	"testing"

	velaruntime "github.com/arcgolabs/vela/runtime"
)

func TestSnapshotBuilderBuildsMatcher(t *testing.T) {
	t.Parallel()

	endpoint, err := velaruntime.NewEndpoint("http://127.0.0.1:8081", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	service := velaruntime.NewService("api", "round_robin", endpoint)
	route := velaruntime.NewRoute("api", "web", service).
		WithHost("API.EXAMPLE.COM").
		WithPathPrefix("/api").
		WithMethod(http.MethodGet)

	snapshot := velaruntime.NewSnapshot().
		AddEntrypoint("web", ":8080", velaruntime.EntrypointRuntime{}).
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

	_, err := velaruntime.NewEndpoint("/api", 1, http.NotFoundHandler())
	if err == nil {
		t.Fatal("NewEndpoint returned nil error for relative URL")
	}
}

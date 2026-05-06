package runtime

import (
	"net/http"
	"testing"
)

func TestSnapshotBuilderBuildsMatcher(t *testing.T) {
	t.Parallel()

	endpoint, err := NewEndpoint("http://127.0.0.1:8081", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService("api", "round_robin", endpoint)
	route := NewRoute("api", "web", service).
		WithHost("API.EXAMPLE.COM").
		WithPathPrefix("/api").
		WithMethod(http.MethodGet)

	snapshot := NewSnapshot().
		AddEntrypoint("web", ":8080", EntrypointRuntime{}).
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

package vale

import (
	"net/http"
	"testing"

	"github.com/arcgolabs/vale/runtime"
)

func TestRootHandlerEndpointBuilder(t *testing.T) {
	t.Parallel()

	endpoint, err := NewHandlerEndpoint("static", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Kind != runtime.EndpointKindHandler || endpoint.Identifier() != "handler:static" {
		t.Fatalf("handler endpoint = %#v, want stable handler identity", endpoint)
	}
}

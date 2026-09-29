package compiler

import (
	"testing"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/runtime"
)

func TestCompileEndpointSetsHTTPKind(t *testing.T) {
	t.Parallel()

	endpoint, err := compileEndpoint("api", config.Endpoint{URL: "http://127.0.0.1:8081"})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Kind != runtime.EndpointKindHTTP {
		t.Fatalf("compiled endpoint kind = %q, want %q", endpoint.Kind, runtime.EndpointKindHTTP)
	}
}

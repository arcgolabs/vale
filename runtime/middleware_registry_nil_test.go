package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestMiddlewareRegistryFallsBackWhenFactoryReturnsTypedNil(t *testing.T) {
	t.Parallel()

	registry := valeruntime.NewMiddlewareRegistry()
	if err := registry.Register("nil", func(http.Handler, valeruntime.MiddlewareRuntime) http.HandlerFunc {
		return nilHandlerFunc()
	}); err != nil {
		t.Fatal(err)
	}
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	factory, ok := registry.Factory("nil")
	if !ok {
		t.Fatal("middleware factory is not registered")
	}
	factory(next, valeruntime.MiddlewareRuntime{}).ServeHTTP(
		httptest.NewRecorder(),
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com", http.NoBody),
	)
	if !called {
		t.Fatal("typed-nil middleware result did not fall back to next handler")
	}
}

func nilHandlerFunc() http.HandlerFunc {
	return nil
}

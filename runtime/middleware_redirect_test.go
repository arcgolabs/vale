package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestWrapMiddlewaresRejectsUnsafeRedirectTarget(t *testing.T) {
	t.Parallel()

	called := false
	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}), collectionlist.NewList[valeruntime.MiddlewareRuntime](
		valeruntime.MiddlewareRuntime{
			RedirectRegex:       `^http://old.example.com/(.*)$`,
			RedirectReplacement: `javascript:alert(1)`,
		},
	))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://old.example.com/docs", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler was called after unsafe redirect")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if location := rec.Header().Get("Location"); location != "" {
		t.Fatalf("location = %q, want empty", location)
	}
}

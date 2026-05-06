package runtime

import (
	"net/http"
	"net/http/httptest"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

func TestWrapMiddlewares(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotHeader string
	handler := WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusNoContent)
	}), collectionlist.NewList[MiddlewareRuntime](
		MiddlewareRuntime{
			StripPrefix: "/api",
			AddPrefix:   "/v1",
			RequestHeaders: mapping.NewMapFrom(map[string]string{
				"X-Test": "ok",
			}),
			ResponseHeaders: mapping.NewMapFrom(map[string]string{
				"X-Response": "set",
			}),
		},
	))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/users", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if gotPath != "/v1/users" {
		t.Fatalf("path = %q, want /v1/users", gotPath)
	}
	if gotHeader != "ok" {
		t.Fatalf("request header = %q, want ok", gotHeader)
	}
	if rec.Header().Get("X-Response") != "set" {
		t.Fatalf("response header = %q, want set", rec.Header().Get("X-Response"))
	}
}

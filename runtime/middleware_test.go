package runtime_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	velaruntime "github.com/arcgolabs/vela/runtime"
)

func TestWrapMiddlewares(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotHeader string
	handler := velaruntime.WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusNoContent)
	}), collectionlist.NewList[velaruntime.MiddlewareRuntime](
		velaruntime.MiddlewareRuntime{
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

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/api/users", http.NoBody)
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

func TestWrapMiddlewaresAppliesPathTransforms(t *testing.T) {
	t.Parallel()

	var gotPath string
	handler := velaruntime.WrapMiddlewares(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
	}), collectionlist.NewList[velaruntime.MiddlewareRuntime](
		velaruntime.MiddlewareRuntime{
			StripPrefixes:          collectionlist.NewList("/api", "/v1"),
			ReplacePathRegex:       `^/users/(.*)$`,
			ReplacePathReplacement: `/accounts/$1`,
			AddPrefix:              "/backend",
		},
	))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/api/v1/users/42", http.NoBody)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if gotPath != "/backend/accounts/42" {
		t.Fatalf("path = %q, want /backend/accounts/42", gotPath)
	}
}

func TestWrapMiddlewaresRedirectsScheme(t *testing.T) {
	t.Parallel()

	called := false
	handler := velaruntime.WrapMiddlewares(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}), collectionlist.NewList[velaruntime.MiddlewareRuntime](
		velaruntime.MiddlewareRuntime{
			RedirectScheme:    "https",
			RedirectPort:      "443",
			RedirectPermanent: true,
		},
	))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/api?q=1", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("handler was called after redirect")
	}
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}
	if location := rec.Header().Get("Location"); location != "https://example.com:443/api?q=1" {
		t.Fatalf("location = %q, want https://example.com:443/api?q=1", location)
	}
}

func TestWrapMiddlewaresRedirectsRegex(t *testing.T) {
	t.Parallel()

	handler := velaruntime.WrapMiddlewares(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler was called after redirect")
	}), collectionlist.NewList[velaruntime.MiddlewareRuntime](
		velaruntime.MiddlewareRuntime{
			RedirectRegex:       `^http://old.example.com/(.*)$`,
			RedirectReplacement: `https://new.example.com/$1`,
		},
	))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://old.example.com/docs", http.NoBody)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusFound)
	}
	if location := rec.Header().Get("Location"); location != "https://new.example.com/docs" {
		t.Fatalf("location = %q, want https://new.example.com/docs", location)
	}
}

func TestMiddlewareRegistryUsesCustomFactory(t *testing.T) {
	t.Parallel()

	registry := velaruntime.NewMiddlewareRegistry()
	if err := registry.Register("mark", func(next http.Handler, middleware velaruntime.MiddlewareRuntime) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Middleware", middleware.Name)
			next.ServeHTTP(w, r)
		})
	}); err != nil {
		t.Fatal(err)
	}

	handler := velaruntime.WrapMiddlewaresWithRegistry(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Middleware"); got != "custom" {
			t.Fatalf("header = %q, want custom", got)
		}
	}), collectionlist.NewList[velaruntime.MiddlewareRuntime](
		velaruntime.MiddlewareRuntime{Name: "custom", Type: "mark"},
	), registry)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody))
}

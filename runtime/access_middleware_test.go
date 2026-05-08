package runtime_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestBuiltinMiddlewareBasicAuth(t *testing.T) {
	t.Parallel()

	called := false
	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), collectionlist.NewList[valeruntime.MiddlewareRuntime](
		valeruntime.MiddlewareRuntime{
			BasicAuth: valeruntime.BasicAuthRuntime{
				Enabled: true,
				Realm:   "private",
				Users:   mapping.NewMapFrom(map[string]string{"admin": "secret"}),
			},
		},
	))

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody))
	if unauthorized.Code != http.StatusUnauthorized || called {
		t.Fatalf("unauthorized status/called = %d/%v, want 401/false", unauthorized.Code, called)
	}

	authorizedReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody)
	authorizedReq.SetBasicAuth("admin", "secret")
	authorized := httptest.NewRecorder()
	handler.ServeHTTP(authorized, authorizedReq)
	if authorized.Code != http.StatusNoContent || !called {
		t.Fatalf("authorized status/called = %d/%v, want 204/true", authorized.Code, called)
	}
}

func TestBuiltinMiddlewareIPAllowList(t *testing.T) {
	t.Parallel()

	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), collectionlist.NewList[valeruntime.MiddlewareRuntime](
		valeruntime.MiddlewareRuntime{
			IPAllowList: valeruntime.IPAllowListRuntime{
				Enabled:            true,
				SourceRange:        collectionlist.NewList("10.0.0.0/8"),
				TrustForwardHeader: true,
			},
		},
	))

	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody))
	if denied.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, want 403", denied.Code)
	}

	allowedReq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody)
	allowedReq.Header.Set("X-Forwarded-For", "10.1.2.3")
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, allowedReq)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("allowed status = %d, want 204", allowed.Code)
	}
}

func TestBuiltinMiddlewareCompressesGzip(t *testing.T) {
	t.Parallel()

	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("hello vale")); err != nil {
			t.Fatal(err)
		}
	}), collectionlist.NewList[valeruntime.MiddlewareRuntime](
		valeruntime.MiddlewareRuntime{
			Compress: valeruntime.CompressRuntime{Enabled: true},
		},
	))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://example.com/", http.NoBody)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("content encoding = %q, want gzip", rec.Header().Get("Content-Encoding"))
	}
	body := readGzipBody(t, rec)
	if string(body) != "hello vale" {
		t.Fatalf("body = %q, want hello vale", body)
	}
}

func readGzipBody(t *testing.T, rec *httptest.ResponseRecorder) []byte {
	t.Helper()

	reader, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeErr := reader.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

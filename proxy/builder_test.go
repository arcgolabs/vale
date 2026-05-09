package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/arcgolabs/vale/proxy"
)

func TestRewriteTargetURLPreservesRequestPathAndQuery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		target     string
		request    string
		wantPath   string
		wantQuery  string
		wantScheme string
		wantHost   string
	}{
		{
			name:       "root target",
			target:     "http://upstream.local",
			request:    "http://gateway.local/api/v1/users?active=true",
			wantPath:   "/api/v1/users",
			wantQuery:  "active=true",
			wantScheme: "http",
			wantHost:   "upstream.local",
		},
		{
			name:       "base path target",
			target:     "http://upstream.local/base",
			request:    "http://gateway.local/api/v1/users?active=true",
			wantPath:   "/base/api/v1/users",
			wantQuery:  "active=true",
			wantScheme: "http",
			wantHost:   "upstream.local",
		},
		{
			name:       "target and request query",
			target:     "http://upstream.local/base?tenant=default",
			request:    "http://gateway.local/api/v1/users?active=true",
			wantPath:   "/base/api/v1/users",
			wantQuery:  "tenant=default&active=true",
			wantScheme: "http",
			wantHost:   "upstream.local",
		},
		{
			name:       "trailing and leading slash",
			target:     "http://upstream.local/base/",
			request:    "http://gateway.local/api/v1/users",
			wantPath:   "/base/api/v1/users",
			wantScheme: "http",
			wantHost:   "upstream.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			targetURL, err := url.Parse(tt.target)
			if err != nil {
				t.Fatal(err)
			}
			requestURL, err := url.Parse(tt.request)
			if err != nil {
				t.Fatal(err)
			}

			got := proxy.RewriteTargetURL(targetURL, requestURL)
			assertRewrittenURL(t, got, tt.wantScheme, tt.wantHost, tt.wantPath, tt.wantQuery)
		})
	}
}

func TestOxyEngineForwardsToTargetHostAndPreservesForwardedHost(t *testing.T) {
	t.Parallel()

	var gotHost string
	var gotForwardedHost string
	var gotPath string
	var gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotForwardedHost = r.Header.Get("X-Forwarded-Host")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	targetURL, err := url.Parse(upstream.URL + "/base?tenant=default")
	if err != nil {
		t.Fatal(err)
	}
	handler := proxy.Build(targetURL)

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "http://gateway.local/api?q=1", http.NoBody)
	req.Host = "gateway.local"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	result := rec.Result()
	defer func() {
		if err := result.Body.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := io.Copy(io.Discard, result.Body); err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", result.StatusCode, http.StatusNoContent)
	}
	if gotHost != targetURL.Host {
		t.Fatalf("upstream host = %q, want %q", gotHost, targetURL.Host)
	}
	if gotForwardedHost != "gateway.local" {
		t.Fatalf("x-forwarded-host = %q, want gateway.local", gotForwardedHost)
	}
	if gotPath != "/base/api" {
		t.Fatalf("upstream path = %q, want /base/api", gotPath)
	}
	if gotQuery != "tenant=default&q=1" {
		t.Fatalf("upstream query = %q, want tenant=default&q=1", gotQuery)
	}
}

func TestRewriteRequestURLMutatesRequestURL(t *testing.T) {
	t.Parallel()

	targetURL, err := url.Parse("http://upstream.local/base?tenant=default")
	if err != nil {
		t.Fatal(err)
	}
	requestURL, err := url.Parse("http://gateway.local/api/v1/users?active=true")
	if err != nil {
		t.Fatal(err)
	}

	proxy.RewriteRequestURL(targetURL, requestURL)

	assertRewrittenURL(t, requestURL, "http", "upstream.local", "/base/api/v1/users", "tenant=default&active=true")
}

func assertRewrittenURL(t *testing.T, got *url.URL, wantScheme, wantHost, wantPath, wantQuery string) {
	t.Helper()
	if got.Scheme != wantScheme {
		t.Fatalf("scheme = %q, want %q", got.Scheme, wantScheme)
	}
	if got.Host != wantHost {
		t.Fatalf("host = %q, want %q", got.Host, wantHost)
	}
	if got.Path != wantPath {
		t.Fatalf("path = %q, want %q", got.Path, wantPath)
	}
	if got.RawQuery != wantQuery {
		t.Fatalf("raw query = %q, want %q", got.RawQuery, wantQuery)
	}
}

package proxy_test

import (
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

package proxy

import (
	"net/url"
	"testing"
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

			got := rewriteTargetURL(targetURL, requestURL)
			if got.Scheme != tt.wantScheme {
				t.Fatalf("scheme = %q, want %q", got.Scheme, tt.wantScheme)
			}
			if got.Host != tt.wantHost {
				t.Fatalf("host = %q, want %q", got.Host, tt.wantHost)
			}
			if got.Path != tt.wantPath {
				t.Fatalf("path = %q, want %q", got.Path, tt.wantPath)
			}
			if got.RawQuery != tt.wantQuery {
				t.Fatalf("raw query = %q, want %q", got.RawQuery, tt.wantQuery)
			}
		})
	}
}

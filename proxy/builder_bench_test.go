package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/arcgolabs/vale/proxy"
)

func BenchmarkOxyEngineProxy(b *testing.B) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	b.Cleanup(upstream.Close)

	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		b.Fatal(err)
	}
	proxyServer := httptest.NewServer(proxy.Build(targetURL))
	b.Cleanup(proxyServer.Close)

	client := proxyServer.Client()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkOxyEngineRequest(b, client, proxyServer.URL+"/api?q=1")
	}
}

func benchmarkOxyEngineRequest(b *testing.B, client *http.Client, targetURL string) {
	b.Helper()

	req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, targetURL, http.NoBody)
	if err != nil {
		b.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		b.Fatal(err)
	}
	defer closeBenchmarkResponseBody(b, resp)
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		b.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		b.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func closeBenchmarkResponseBody(b *testing.B, resp *http.Response) {
	b.Helper()

	if err := resp.Body.Close(); err != nil {
		b.Fatal(err)
	}
}

package proxy_test

import (
	"net/http"
	"testing"

	"github.com/arcgolabs/vale/proxy"
)

func TestNewOxyTransportKeepsReusableUpstreamConnections(t *testing.T) {
	t.Parallel()

	transport := proxy.NewOxyTransport()
	if transport.MaxIdleConns < 1024 {
		t.Fatalf("MaxIdleConns = %d, want at least 1024", transport.MaxIdleConns)
	}
	if transport.MaxIdleConnsPerHost < 256 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want at least 256", transport.MaxIdleConnsPerHost)
	}
	if transport.Proxy == nil {
		t.Fatal("Proxy is nil, want default proxy lookup preserved")
	}
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("http.DefaultTransport is not *http.Transport")
	}
	if defaultTransport.MaxIdleConnsPerHost == transport.MaxIdleConnsPerHost {
		t.Fatal("transport should override the Go default per-host idle connection limit")
	}
}

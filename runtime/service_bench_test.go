package runtime_test

import (
	"net/url"
	"testing"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func BenchmarkServiceRuntimePickSingleEndpoint(b *testing.B) {
	endpointURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		b.Fatal(err)
	}
	endpoint := &valeruntime.EndpointRuntime{URL: endpointURL, Weight: 1}
	endpoint.Healthy.Store(true)
	service := valeruntime.NewService("api", "round_robin", endpoint)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		got, err := service.Pick()
		if err != nil {
			b.Fatal(err)
		}
		if got != endpoint {
			b.Fatalf("picked endpoint = %v, want single endpoint", got)
		}
	}
}

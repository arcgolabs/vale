package runtime_test

import (
	"net/url"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
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

func BenchmarkServiceRuntimePickWeightedRoundRobin(b *testing.B) {
	firstURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		b.Fatal(err)
	}
	secondURL, err := url.Parse("http://127.0.0.1:8082")
	if err != nil {
		b.Fatal(err)
	}
	thirdURL, err := url.Parse("http://127.0.0.1:8083")
	if err != nil {
		b.Fatal(err)
	}

	service := &valeruntime.ServiceRuntime{
		Name:     "api",
		Strategy: "weighted_round_robin",
		Endpoints: collectionlist.NewList[*valeruntime.EndpointRuntime](
			&valeruntime.EndpointRuntime{URL: firstURL, Weight: 10000},
			&valeruntime.EndpointRuntime{URL: secondURL, Weight: 1000},
			&valeruntime.EndpointRuntime{URL: thirdURL, Weight: 10},
		),
	}
	service.Endpoints.Range(func(_ int, endpoint *valeruntime.EndpointRuntime) bool {
		endpoint.Healthy.Store(true)
		return true
	})
	service.BuildSlots()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := service.Pick(); err != nil {
			b.Fatal(err)
		}
	}
}

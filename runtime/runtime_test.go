package runtime

import (
	"net/url"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func TestServiceRuntimePickSkipsUnhealthyEndpoints(t *testing.T) {
	t.Parallel()

	healthyURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	unhealthyURL, err := url.Parse("http://127.0.0.1:8082")
	if err != nil {
		t.Fatal(err)
	}

	service := &ServiceRuntime{
		Name:     "api",
		Strategy: "round_robin",
		Endpoints: collectionlist.NewList[*EndpointRuntime](
			&EndpointRuntime{URL: unhealthyURL, Weight: 1},
			&EndpointRuntime{URL: healthyURL, Weight: 1},
		),
	}
	unhealthy, _ := service.Endpoints.Get(0)
	healthy, _ := service.Endpoints.Get(1)
	unhealthy.Healthy.Store(false)
	healthy.Healthy.Store(true)

	for i := 0; i < 4; i++ {
		got, err := service.Pick()
		if err != nil {
			t.Fatal(err)
		}
		if got.URL.String() != healthyURL.String() {
			t.Fatalf("picked endpoint = %s, want %s", got.URL.String(), healthyURL.String())
		}
	}
}

func TestServiceRuntimeWeightedRoundRobinUsesWeights(t *testing.T) {
	t.Parallel()

	firstURL, err := url.Parse("http://127.0.0.1:8081")
	if err != nil {
		t.Fatal(err)
	}
	secondURL, err := url.Parse("http://127.0.0.1:8082")
	if err != nil {
		t.Fatal(err)
	}

	service := &ServiceRuntime{
		Name:     "api",
		Strategy: "weighted_round_robin",
		Endpoints: collectionlist.NewList[*EndpointRuntime](
			&EndpointRuntime{URL: firstURL, Weight: 2},
			&EndpointRuntime{URL: secondURL, Weight: 1},
		),
	}
	service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
		endpoint.Healthy.Store(true)
		return true
	})
	service.BuildSlots()

	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		got, err := service.Pick()
		if err != nil {
			t.Fatal(err)
		}
		counts[got.URL.String()]++
	}

	if counts[firstURL.String()] != 4 {
		t.Fatalf("first endpoint picks = %d, want 4", counts[firstURL.String()])
	}
	if counts[secondURL.String()] != 2 {
		t.Fatalf("second endpoint picks = %d, want 2", counts[secondURL.String()])
	}
}

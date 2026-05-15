package k8s_test

import (
	"context"
	"sync/atomic"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	providerk8s "github.com/arcgolabs/vale/provider/k8s"
)

func TestMemorySourceListEndpointsReturnsSnapshot(t *testing.T) {
	t.Parallel()

	source := providerk8s.NewMemorySource(
		nil,
		collectionlist.NewList(providerk8s.ServiceEndpoint{
			Service: "api",
			URL:     "http://10.0.0.2",
			Weight:  1,
		}),
	)

	endpoints, err := source.ListEndpoints(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	endpoints.Add(providerk8s.ServiceEndpoint{Service: "hack"})

	current, err := source.ListEndpoints(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if current.Len() != 1 {
		t.Fatalf("expected snapshot isolation, got len=%d", current.Len())
	}
}

func TestMemorySourceWatchNotifiesOncePerUpdate(t *testing.T) {
	t.Parallel()

	source := providerk8s.NewMemorySource(
		collectionlist.NewList(providerk8s.HTTPRoute{Name: "route"}),
		collectionlist.NewList(providerk8s.ServiceEndpoint{Service: "svc", URL: "http://10.0.0.1"}),
	)
	var calls atomic.Int32

	closer, err := source.Watch(context.Background(), func() {
		calls.Add(1)
	}, func(error) {})
	if err != nil {
		t.Fatalf("watch failed: %v", err)
	}
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Fatalf("close error: %v", err)
		}
	})

	source.Update(
		collectionlist.NewList(providerk8s.HTTPRoute{Name: "route-updated"}),
		collectionlist.NewList(providerk8s.ServiceEndpoint{Service: "svc", URL: "http://10.0.0.2"}),
	)
	if calls.Load() != 1 {
		t.Fatalf("expected 1 notification, got %d", calls.Load())
	}
}

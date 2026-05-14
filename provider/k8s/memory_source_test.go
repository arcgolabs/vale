package k8s

import (
	"context"
	"sync/atomic"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func TestMemorySourceListEndpointsReturnsSnapshot(t *testing.T) {
	t.Parallel()

	source := NewMemorySource(
		nil,
		collectionlist.NewList(ServiceEndpoint{
			Service: "api",
			URL:     "http://10.0.0.2",
			Weight:  1,
		}),
	)

	endpoints, err := source.ListEndpoints(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	endpoints.Add(ServiceEndpoint{Service: "hack"})

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

	source := NewMemorySource(
		collectionlist.NewList(HTTPRoute{Name: "route"}),
		collectionlist.NewList(ServiceEndpoint{Service: "svc", URL: "http://10.0.0.1"}),
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
		collectionlist.NewList(HTTPRoute{Name: "route-updated"}),
		collectionlist.NewList(ServiceEndpoint{Service: "svc", URL: "http://10.0.0.2"}),
	)
	if calls.Load() != 1 {
		t.Fatalf("expected 1 notification, got %d", calls.Load())
	}
}

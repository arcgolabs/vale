package provider_test

import (
	"sync/atomic"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/provider"
)

func TestStateStoreLoadUsesSnapshot(t *testing.T) {
	t.Parallel()

	store := provider.NewStateStore(
		collectionlist.NewList("a", "b"),
		func(input *collectionlist.List[string]) *collectionlist.List[string] {
			if input == nil {
				return collectionlist.NewList[string]()
			}
			return input.Clone()
		},
	)

	closer := store.Watch(func() {})
	if err := closer.Close(); err != nil {
		t.Fatalf("close error: %v", err)
	}

	list := store.Load()
	list.Add("x")

	loaded := store.Load()
	if loaded.Len() != 2 {
		t.Fatalf("snapshot leaked mutation, len=%d", loaded.Len())
	}
}

func TestStateStoreUpdateNotifiesWatchers(t *testing.T) {
	t.Parallel()

	store := provider.NewStateStore("v1", nil)
	var calls atomic.Int32

	closer := store.Watch(func() {
		calls.Add(1)
	})
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Fatalf("close error: %v", err)
		}
	})

	store.Update("v2")
	store.Update("v3")

	if calls.Load() != 2 {
		t.Fatalf("expected 2 notifications, got %d", calls.Load())
	}
}

func TestStateStoreSetDoesNotNotifyWatchers(t *testing.T) {
	t.Parallel()

	store := provider.NewStateStore("v1", nil)
	var calls atomic.Int32

	closer := store.Watch(func() {
		calls.Add(1)
	})
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Fatalf("close error: %v", err)
		}
	})

	store.Set("v2")
	if calls.Load() != 0 {
		t.Fatalf("expected no notifications, got %d", calls.Load())
	}
}

func TestStateStoreLoadOnNilReturnsZeroValue(t *testing.T) {
	t.Parallel()
	t.Run("nil store", func(t *testing.T) {
		var store *provider.StateStore[string]
		if got := store.Load(); got != "" {
			t.Fatalf("expected zero value, got %q", got)
		}
	})
	t.Run("watch on nil store", func(t *testing.T) {
		var store *provider.StateStore[string]
		closer := store.Watch(func() {})
		if closer == nil {
			t.Fatalf("expected non-nil closer")
		}
		if err := closer.Close(); err != nil {
			t.Fatalf("close error: %v", err)
		}
	})
}

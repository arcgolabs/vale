package docker_test

import (
	"context"
	"sync/atomic"
	"testing"

	providerdocker "github.com/arcgolabs/vale/provider/docker"
)

func TestMemorySourceListContainersReturnsSnapshot(t *testing.T) {
	t.Parallel()

	source := providerdocker.NewMemorySource(providerdocker.Container{Name: "api"})
	containers, err := source.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	containers.Add(providerdocker.Container{Name: "hack"})
	current, err := source.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if current.Len() != 1 {
		t.Fatalf("expected snapshot isolation, got len=%d", current.Len())
	}
}

func TestMemorySourceWatchNotifiesOncePerUpdate(t *testing.T) {
	t.Parallel()

	source := providerdocker.NewMemorySource(providerdocker.Container{Name: "api"})
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

	source.Update(providerdocker.Container{Name: "api2"})
	source.Update(providerdocker.Container{Name: "api3"}, providerdocker.Container{Name: "api4"})
	if calls.Load() != 2 {
		t.Fatalf("expected 2 notifications, got %d", calls.Load())
	}
}

func TestMemorySourceUpdateCanAcceptNoArgs(t *testing.T) {
	t.Parallel()

	source := providerdocker.NewMemorySource(providerdocker.Container{Name: "api"})
	source.Update()

	containers, err := source.ListContainers(context.Background())
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if containers.Len() != 0 {
		t.Fatalf("expected empty list, got %d", containers.Len())
	}
}

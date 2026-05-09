package docker

import (
	"context"
	"io"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/provider"
)

type MemorySource struct {
	mu         sync.RWMutex
	containers *collectionlist.List[Container]
	watchHub   *provider.WatchHub
}

func NewMemorySource(containers ...Container) *MemorySource {
	return &MemorySource{
		containers: collectionlist.NewList(containers...),
		watchHub:   provider.NewWatchHub(),
	}
}

func (s *MemorySource) ListContainers(_ context.Context) (*collectionlist.List[Container], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.containers.Clone(), nil
}

func (s *MemorySource) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	return s.watchHub.Watch(onReload), nil
}

func (s *MemorySource) Update(containers ...Container) {
	s.mu.Lock()
	s.containers = collectionlist.NewList(containers...)
	s.mu.Unlock()

	s.watchHub.Notify()
}

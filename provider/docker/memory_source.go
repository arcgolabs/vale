package docker

import (
	"context"
	"io"
	"sync"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/provider"
)

type MemorySource struct {
	mu         sync.RWMutex
	containers []Container
	listeners  *mapping.Map[int, func()]
	nextID     int
}

func NewMemorySource(containers ...Container) *MemorySource {
	return &MemorySource{
		containers: containers,
		listeners:  mapping.NewMap[int, func()](),
	}
}

func (s *MemorySource) ListContainers(_ context.Context) ([]Container, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Container, len(s.containers))
	copy(out, s.containers)
	return out, nil
}

func (s *MemorySource) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	s.listeners.Set(id, onReload)
	return provider.NewOnceCloser(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.listeners.Delete(id)
	}), nil
}

func (s *MemorySource) Update(containers ...Container) {
	s.mu.Lock()
	s.containers = containers
	listeners := s.listeners.Values()
	s.mu.Unlock()

	for _, listener := range listeners {
		if listener != nil {
			listener()
		}
	}
}

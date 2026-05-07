package k8s

import (
	"context"
	"io"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/provider"
)

type MemorySource struct {
	mu        sync.RWMutex
	routes    *collectionlist.List[HTTPRoute]
	endpoints *collectionlist.List[ServiceEndpoint]
	listeners *mapping.Map[int, func()]
	nextID    int
}

func NewMemorySource(routes *collectionlist.List[HTTPRoute], endpoints *collectionlist.List[ServiceEndpoint]) *MemorySource {
	return &MemorySource{
		routes:    routes,
		endpoints: endpoints,
		listeners: mapping.NewMap[int, func()](),
	}
}

func (s *MemorySource) ListRoutes(_ context.Context) (*collectionlist.List[HTTPRoute], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.routes.Clone(), nil
}

func (s *MemorySource) ListEndpoints(_ context.Context) (*collectionlist.List[ServiceEndpoint], error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.endpoints.Clone(), nil
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

func (s *MemorySource) Update(routes *collectionlist.List[HTTPRoute], endpoints *collectionlist.List[ServiceEndpoint]) {
	s.mu.Lock()
	s.routes = routes
	s.endpoints = endpoints
	listeners := s.listeners.Values()
	s.mu.Unlock()

	collectionlist.NewList(listeners...).Range(func(_ int, listener func()) bool {
		if listener != nil {
			listener()
		}
		return true
	})
}

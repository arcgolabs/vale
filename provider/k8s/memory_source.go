package k8s

import (
	"context"
	"io"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/provider"
)

type MemorySource struct {
	mu        sync.RWMutex
	routes    *collectionlist.List[HTTPRoute]
	endpoints *collectionlist.List[ServiceEndpoint]
	watchHub  *provider.WatchHub
}

func NewMemorySource(routes *collectionlist.List[HTTPRoute], endpoints *collectionlist.List[ServiceEndpoint]) *MemorySource {
	return &MemorySource{
		routes:    routes,
		endpoints: endpoints,
		watchHub:  provider.NewWatchHub(),
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
	return s.watchHub.Watch(onReload), nil
}

func (s *MemorySource) Update(routes *collectionlist.List[HTTPRoute], endpoints *collectionlist.List[ServiceEndpoint]) {
	s.mu.Lock()
	s.routes = routes
	s.endpoints = endpoints
	s.mu.Unlock()

	s.watchHub.Notify()
}

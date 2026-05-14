package k8s

import (
	"context"
	"io"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/provider"
)

type MemorySource struct {
	watchHub  *provider.WatchHub
	routes    *provider.StateStore[*collectionlist.List[HTTPRoute]]
	endpoints *provider.StateStore[*collectionlist.List[ServiceEndpoint]]
}

func NewMemorySource(routes *collectionlist.List[HTTPRoute], endpoints *collectionlist.List[ServiceEndpoint]) *MemorySource {
	if routes == nil {
		routes = collectionlist.NewList[HTTPRoute]()
	}
	if endpoints == nil {
		endpoints = collectionlist.NewList[ServiceEndpoint]()
	}

	return &MemorySource{
		watchHub: provider.NewWatchHub(),
		routes: provider.NewStateStore(routes, func(routes *collectionlist.List[HTTPRoute]) *collectionlist.List[HTTPRoute] {
			if routes == nil {
				return collectionlist.NewList[HTTPRoute]()
			}
			return routes.Clone()
		}),
		endpoints: provider.NewStateStore(endpoints, func(endpoints *collectionlist.List[ServiceEndpoint]) *collectionlist.List[ServiceEndpoint] {
			if endpoints == nil {
				return collectionlist.NewList[ServiceEndpoint]()
			}
			return endpoints.Clone()
		}),
	}
}

func (s *MemorySource) ListRoutes(_ context.Context) (*collectionlist.List[HTTPRoute], error) {
	return s.routes.Load(), nil
}

func (s *MemorySource) ListEndpoints(_ context.Context) (*collectionlist.List[ServiceEndpoint], error) {
	return s.endpoints.Load(), nil
}

func (s *MemorySource) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	return s.watchHub.Watch(onReload), nil
}

func (s *MemorySource) Update(routes *collectionlist.List[HTTPRoute], endpoints *collectionlist.List[ServiceEndpoint]) {
	s.routes.Set(routes)
	s.endpoints.Set(endpoints)
	s.watchHub.Notify()
}

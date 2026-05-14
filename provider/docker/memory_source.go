package docker

import (
	"context"
	"io"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/provider"
)

type MemorySource struct {
	containers *provider.StateStore[*collectionlist.List[Container]]
}

func NewMemorySource(containers ...Container) *MemorySource {
	return &MemorySource{
		containers: provider.NewStateStore(
			collectionlist.NewList(containers...),
			func(containers *collectionlist.List[Container]) *collectionlist.List[Container] {
				if containers == nil {
					return collectionlist.NewList[Container]()
				}
				return containers.Clone()
			},
		),
	}
}

func (s *MemorySource) ListContainers(_ context.Context) (*collectionlist.List[Container], error) {
	return s.containers.Load(), nil
}

func (s *MemorySource) Watch(_ context.Context, onReload func(), _ func(error)) (io.Closer, error) {
	return s.containers.Watch(onReload), nil
}

func (s *MemorySource) Update(containers ...Container) {
	s.containers.Update(collectionlist.NewList(containers...))
}

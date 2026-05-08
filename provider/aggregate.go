// Package provider defines Vale snapshot/config provider interfaces and helpers.
package provider

import (
	"context"
	"errors"
	"fmt"
	"io"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/runtime"
)

type fallbackProvider struct {
	providers *collectionlist.List[SnapshotProvider]
}

func Fallback(providers ...SnapshotProvider) SnapshotProvider {
	return FallbackList(collectionlist.NewList(providers...))
}

func FallbackList(providers *collectionlist.List[SnapshotProvider]) SnapshotProvider {
	nonNilProviders := collectionlist.FilterList(providers, func(_ int, p SnapshotProvider) bool {
		return p != nil
	})
	return &fallbackProvider{providers: nonNilProviders}
}

func (p *fallbackProvider) Load(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	if p.providers.IsEmpty() {
		return nil, errors.New("fallback provider has no providers")
	}

	messages := collectionlist.NewList[string]()
	var loaded *runtime.CompiledSnapshot
	p.providers.Range(func(index int, current SnapshotProvider) bool {
		snapshot, err := current.Load(ctx)
		if err == nil {
			loaded = snapshot
			return false
		}
		messages.Add(fmt.Sprintf("provider[%d]: %v", index, err))
		return true
	})
	if loaded != nil {
		return loaded, nil
	}
	return nil, fmt.Errorf("all providers failed: %s", messages.Join("; "))
}

func (p *fallbackProvider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	if p.providers.IsEmpty() {
		return nil, errors.New("fallback provider has no providers")
	}
	closers := collectionlist.NewListWithCapacity[io.Closer](p.providers.Len())
	var setupErr error
	p.providers.Range(func(index int, current SnapshotProvider) bool {
		closer, err := current.Watch(ctx, onReload, func(err error) {
			onError(fmt.Errorf("provider[%d] watch error: %w", index, err))
		})
		if err != nil {
			closeErr := NewMultiCloser(closers).Close()
			setupErr = errors.Join(fmt.Errorf("provider[%d] watch setup failed: %w", index, err), closeErr)
			return false
		}
		closers.Add(closer)
		return true
	})
	if setupErr != nil {
		return nil, setupErr
	}
	return NewMultiCloser(closers), nil
}

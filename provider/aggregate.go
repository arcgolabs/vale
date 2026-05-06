package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/runtime"
)

type fallbackProvider struct {
	providers *collectionlist.List[SnapshotProvider]
}

func Fallback(providers ...SnapshotProvider) SnapshotProvider {
	nonNilProviders := collectionlist.NewListWithCapacity[SnapshotProvider](len(providers))
	for _, p := range providers {
		if p != nil {
			nonNilProviders.Add(p)
		}
	}
	return &fallbackProvider{providers: nonNilProviders}
}

func (p *fallbackProvider) Load(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	if p.providers.IsEmpty() {
		return nil, fmt.Errorf("fallback provider has no providers")
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
	return nil, fmt.Errorf("all providers failed: %s", strings.Join(messages.Values(), "; "))
}

func (p *fallbackProvider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	if p.providers.IsEmpty() {
		return nil, fmt.Errorf("fallback provider has no providers")
	}
	closers := collectionlist.NewListWithCapacity[io.Closer](p.providers.Len())
	var setupErr error
	p.providers.Range(func(index int, current SnapshotProvider) bool {
		closer, err := current.Watch(ctx, onReload, func(err error) {
			onError(fmt.Errorf("provider[%d] watch error: %w", index, err))
		})
		if err != nil {
			closers.Range(func(_ int, c io.Closer) bool {
				_ = c.Close()
				return true
			})
			setupErr = fmt.Errorf("provider[%d] watch setup failed: %w", index, err)
			return false
		}
		closers.Add(closer)
		return true
	})
	if setupErr != nil {
		return nil, setupErr
	}
	return MultiCloser(closers.Values()), nil
}

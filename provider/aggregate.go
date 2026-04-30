package provider

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/arcgolabs/gateway/runtime"
)

type fallbackProvider struct {
	providers []SnapshotProvider
}

func Fallback(providers ...SnapshotProvider) SnapshotProvider {
	nonNilProviders := make([]SnapshotProvider, 0, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		nonNilProviders = append(nonNilProviders, p)
	}
	return &fallbackProvider{providers: nonNilProviders}
}

func (p *fallbackProvider) Load(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	if len(p.providers) == 0 {
		return nil, fmt.Errorf("fallback provider has no providers")
	}

	var messages []string
	for index, current := range p.providers {
		snapshot, err := current.Load(ctx)
		if err == nil {
			return snapshot, nil
		}
		messages = append(messages, fmt.Sprintf("provider[%d]: %v", index, err))
	}
	return nil, fmt.Errorf("all providers failed: %s", strings.Join(messages, "; "))
}

func (p *fallbackProvider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	if len(p.providers) == 0 {
		return nil, fmt.Errorf("fallback provider has no providers")
	}
	closers := make([]io.Closer, 0, len(p.providers))
	for index, current := range p.providers {
		closer, err := current.Watch(ctx, onReload, func(err error) {
			onError(fmt.Errorf("provider[%d] watch error: %w", index, err))
		})
		if err != nil {
			for _, c := range closers {
				_ = c.Close()
			}
			return nil, fmt.Errorf("provider[%d] watch setup failed: %w", index, err)
		}
		closers = append(closers, closer)
	}
	return multiCloser(closers), nil
}

type multiCloser []io.Closer

func (m multiCloser) Close() error {
	var firstErr error
	for _, closer := range m {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

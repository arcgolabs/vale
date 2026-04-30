package merged

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/gateway/compiler"
	"github.com/arcgolabs/gateway/config"
	"github.com/arcgolabs/gateway/provider"
	"github.com/arcgolabs/gateway/runtime"
)

type Source struct {
	Name     string
	Provider provider.ConfigProvider
}

type Provider struct {
	sources *mapping.OrderedMap[string, provider.ConfigProvider]
	bus     eventx.BusRuntime
}

func New(bus eventx.BusRuntime, sources ...Source) *Provider {
	orderedSources := mapping.NewOrderedMap[string, provider.ConfigProvider]()
	for index, source := range sources {
		if source.Provider == nil {
			continue
		}
		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = provider.ConfigProviderName(source.Provider, fmt.Sprintf("source-%d", index))
		}
		orderedSources.Set(name, source.Provider)
	}
	return &Provider{
		sources: orderedSources,
		bus:     bus,
	}
}

func (p *Provider) Load(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	cfg, err := p.loadMergedConfig(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, compileErr := compiler.Compile(cfg)
	if compileErr != nil {
		return nil, compileErr
	}
	p.publish(ctx, provider.SnapshotRecompiledEvent{
		SourceCount:  p.sources.Len(),
		RouteCount:   len(snapshot.Routes()),
		ServiceCount: len(snapshot.ServicesView()),
	})
	return snapshot, nil
}

func (p *Provider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	if p.sources == nil || p.sources.Len() == 0 {
		return nil, fmt.Errorf("merged provider has no config providers")
	}

	closers := make([]io.Closer, 0, p.sources.Len())
	setupFailed := false
	reload := func(sourceName string) {
		p.publish(ctx, provider.ConfigSourceChangedEvent{Source: sourceName})
		snapshot, err := p.Load(ctx)
		if err != nil {
			onError(err)
			return
		}
		onReload(snapshot)
	}

	p.sources.Range(func(sourceName string, configProvider provider.ConfigProvider) bool {
		closer, err := configProvider.Watch(ctx, func() { reload(sourceName) }, func(err error) {
			onError(fmt.Errorf("config provider[%s] watch error: %w", sourceName, err))
		})
		if err != nil {
			p.publish(ctx, provider.WatchSetupFailedEvent{
				Source: sourceName,
				Error:  err.Error(),
			})
			for _, c := range closers {
				_ = c.Close()
			}
			onError(fmt.Errorf("config provider[%s] watch setup failed: %w", sourceName, err))
			setupFailed = true
			return false
		}
		closers = append(closers, closer)
		return true
	})
	if setupFailed || len(closers) == 0 {
		return nil, fmt.Errorf("merged provider failed to setup any watcher")
	}
	return multiCloser(closers), nil
}

func (p *Provider) loadMergedConfig(ctx context.Context) (*config.Config, error) {
	if p.sources == nil || p.sources.Len() == 0 {
		return nil, fmt.Errorf("merged provider has no config providers")
	}

	loadedConfigs := make([]*config.Config, 0, p.sources.Len())
	messages := make([]string, 0)

	p.sources.Range(func(sourceName string, configProvider provider.ConfigProvider) bool {
		start := time.Now()
		cfg, err := configProvider.Load(ctx)
		if err != nil {
			messages = append(messages, fmt.Sprintf("config provider[%s] load failed: %v", sourceName, err))
			p.publish(ctx, provider.ConfigSourceFailedEvent{
				Source: sourceName,
				Error:  err.Error(),
			})
			return true
		}
		p.publish(ctx, provider.ConfigSourceLoadedEvent{
			Source:     sourceName,
			Duration:   time.Since(start),
			ConfigSize: len(cfg.Entrypoints) + len(cfg.Services) + len(cfg.Routes),
		})
		loadedConfigs = append(loadedConfigs, cfg)
		return true
	})

	if len(loadedConfigs) == 0 {
		return nil, fmt.Errorf("failed to load any config: %s", strings.Join(messages, "; "))
	}

	merged := config.Merge(loadedConfigs...)
	if err := config.Validate(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func (p *Provider) publish(ctx context.Context, event eventx.Event) {
	if p.bus == nil || event == nil {
		return
	}
	_ = p.bus.Publish(ctx, event)
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

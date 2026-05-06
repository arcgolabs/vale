package merged

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/compiler"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	"github.com/arcgolabs/vela/runtime"
)

type Source struct {
	Name     string
	Provider provider.ConfigProvider
}

type Provider struct {
	sources *mapping.OrderedMap[string, provider.ConfigProvider]
	bus     provider.EventBus
	logger  *slog.Logger
}

func New(bus provider.EventBus, sources ...Source) *Provider {
	return NewWithLogger(bus, nil, sources...)
}

func NewWithLogger(bus provider.EventBus, logger *slog.Logger, sources ...Source) *Provider {
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
		logger:  logger,
	}
}

func (p *Provider) SetLogger(logger *slog.Logger) {
	p.logger = logger
}

func (p *Provider) Load(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	sourceCount := p.sourceCount()
	if p.logger != nil {
		p.logger.Info("loading merged config", "sources", sourceCount)
	}
	cfg, err := p.loadMergedConfig(ctx)
	if err != nil {
		return nil, err
	}
	snapshot, compileErr := compiler.Compile(cfg)
	if compileErr != nil {
		if p.logger != nil {
			p.logger.Error("compile merged config failed", "error", compileErr)
		}
		return nil, compileErr
	}
	if p.logger != nil {
		p.logger.Info("merged snapshot compiled",
			"built_at", snapshot.BuiltAt,
			"entrypoints", snapshot.Entrypoints.Len(),
			"services", snapshot.Services.Len(),
			"routes", snapshot.Routes().Len(),
		)
	}
	p.publish(ctx, provider.SnapshotRecompiledEvent{
		SourceCount:  sourceCount,
		RouteCount:   snapshot.Routes().Len(),
		ServiceCount: snapshot.ServicesView().Len(),
	})
	return snapshot, nil
}

func (p *Provider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	if p.sources == nil || p.sources.Len() == 0 {
		return nil, fmt.Errorf("merged provider has no config providers")
	}

	closers := collectionlist.NewListWithCapacity[io.Closer](p.sources.Len())
	setupFailed := false
	reload := func(sourceName string) {
		if p.logger != nil {
			p.logger.Info("config source changed", "source", sourceName)
		}
		p.publish(ctx, provider.ConfigSourceChangedEvent{Source: sourceName})
		snapshot, err := p.Load(ctx)
		if err != nil {
			if p.logger != nil {
				p.logger.Error("reload merged snapshot failed", "source", sourceName, "error", err)
			}
			onError(err)
			return
		}
		if p.logger != nil {
			p.logger.Info("merged snapshot reloaded", "source", sourceName, "built_at", snapshot.BuiltAt)
		}
		onReload(snapshot)
	}

	p.sources.Range(func(sourceName string, configProvider provider.ConfigProvider) bool {
		if p.logger != nil {
			p.logger.Info("watching config source", "source", sourceName)
		}
		closer, err := configProvider.Watch(ctx, func() { reload(sourceName) }, func(err error) {
			onError(fmt.Errorf("config provider[%s] watch error: %w", sourceName, err))
		})
		if err != nil {
			if p.logger != nil {
				p.logger.Error("config source watch setup failed", "source", sourceName, "error", err)
			}
			p.publish(ctx, provider.WatchSetupFailedEvent{
				Source: sourceName,
				Error:  err.Error(),
			})
			closers.Range(func(_ int, c io.Closer) bool {
				_ = c.Close()
				return true
			})
			onError(fmt.Errorf("config provider[%s] watch setup failed: %w", sourceName, err))
			setupFailed = true
			return false
		}
		if p.logger != nil {
			p.logger.Info("config source watcher ready", "source", sourceName)
		}
		closers.Add(closer)
		return true
	})
	if setupFailed || closers.IsEmpty() {
		return nil, fmt.Errorf("merged provider failed to setup any watcher")
	}
	return provider.MultiCloser(closers.Values()), nil
}

func (p *Provider) loadMergedConfig(ctx context.Context) (*config.Config, error) {
	if p.sources == nil || p.sources.Len() == 0 {
		return nil, fmt.Errorf("merged provider has no config providers")
	}

	loadedConfigs := collectionlist.NewListWithCapacity[*config.Config](p.sources.Len())
	messages := collectionlist.NewList[string]()

	p.sources.Range(func(sourceName string, configProvider provider.ConfigProvider) bool {
		start := time.Now()
		cfg, err := configProvider.Load(ctx)
		if err != nil {
			messages.Add(fmt.Sprintf("config provider[%s] load failed: %v", sourceName, err))
			if p.logger != nil {
				p.logger.Error("config source load failed", "source", sourceName, "error", err)
			}
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
		if p.logger != nil {
			p.logger.Info("config source loaded",
				"source", sourceName,
				"duration", time.Since(start),
				"entrypoints", len(cfg.Entrypoints),
				"services", len(cfg.Services),
				"routes", len(cfg.Routes),
			)
		}
		loadedConfigs.Add(cfg)
		return true
	})

	if loadedConfigs.IsEmpty() {
		return nil, fmt.Errorf("failed to load any config: %s", strings.Join(messages.Values(), "; "))
	}

	merged := config.Merge(loadedConfigs.Values()...)
	if err := config.Validate(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

func (p *Provider) publish(ctx context.Context, event provider.Event) {
	if p.bus == nil || event == nil {
		return
	}
	_ = p.bus.Publish(ctx, event)
}

func (p *Provider) sourceCount() int {
	if p == nil || p.sources == nil {
		return 0
	}
	return p.sources.Len()
}

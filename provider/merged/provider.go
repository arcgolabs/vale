package merged

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/compiler"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	"github.com/arcgolabs/vela/runtime"
	"github.com/samber/oops"
)

type Source struct {
	Name     string
	Provider provider.ConfigProvider
}

type Provider struct {
	sources         *mapping.OrderedMap[string, provider.ConfigProvider]
	bus             provider.EventBus
	logger          *slog.Logger
	mu              sync.Mutex
	lastFingerprint string
	reloadDebounce  time.Duration
}

func New(bus provider.EventBus, sources ...Source) *Provider {
	return NewWithLogger(bus, nil, sources...)
}

func NewWithLogger(bus provider.EventBus, logger *slog.Logger, sources ...Source) *Provider {
	return NewWithLoggerList(bus, logger, collectionlist.NewList(sources...))
}

func NewWithLoggerList(bus provider.EventBus, logger *slog.Logger, sources *collectionlist.List[Source]) *Provider {
	orderedSources := mapping.NewOrderedMap[string, provider.ConfigProvider]()
	sources.Range(func(index int, source Source) bool {
		if source.Provider == nil {
			return true
		}
		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = provider.ConfigProviderName(source.Provider, fmt.Sprintf("source-%d", index))
		}
		orderedSources.Set(name, source.Provider)
		return true
	})
	return &Provider{
		sources:        orderedSources,
		bus:            bus,
		logger:         logger,
		reloadDebounce: 200 * time.Millisecond,
	}
}

func (p *Provider) SetLogger(logger *slog.Logger) {
	p.logger = logger
}

func (p *Provider) SetReloadDebounce(duration time.Duration) {
	p.reloadDebounce = duration
}

func (p *Provider) Load(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	sourceCount := p.sourceCount()
	if p.logger != nil {
		p.logger.Info("loading merged config", "sources", sourceCount)
	}
	snapshot, fingerprint, compileErr := p.loadSnapshot(ctx)
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
	p.storeFingerprint(fingerprint)
	p.publish(ctx, provider.SnapshotRecompiledEvent{
		SourceCount:  sourceCount,
		RouteCount:   snapshot.Routes().Len(),
		ServiceCount: snapshot.ServicesView().Len(),
		Fingerprint:  fingerprint,
	})
	return snapshot, nil
}

func (p *Provider) loadSnapshot(ctx context.Context) (*runtime.CompiledSnapshot, string, error) {
	cfg, err := p.loadMergedConfig(ctx)
	if err != nil {
		return nil, "", err
	}
	fingerprint, err := config.Fingerprint(cfg)
	if err != nil {
		return nil, "", oops.In("provider.merged").Wrapf(err, "fingerprint merged config")
	}
	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		return nil, "", oops.In("provider.merged").Wrapf(err, "compile merged config")
	}
	return snapshot, fingerprint, nil
}

func (p *Provider) loadMergedConfig(ctx context.Context) (*config.Config, error) {
	if p.sources == nil || p.sources.Len() == 0 {
		return nil, errors.New("merged provider has no config providers")
	}

	loadedConfigs := collectionlist.NewListWithCapacity[*config.Config](p.sources.Len())
	messages := collectionlist.NewList[string]()

	p.sources.Range(func(sourceName string, configProvider provider.ConfigProvider) bool {
		p.loadSourceConfig(ctx, sourceName, configProvider, loadedConfigs, messages)
		return true
	})

	if loadedConfigs.IsEmpty() {
		return nil, fmt.Errorf("failed to load any config: %s", messages.Join("; "))
	}

	merged := config.MergeList(loadedConfigs)
	if err := config.Validate(merged); err != nil {
		return nil, oops.In("provider.merged").Wrapf(err, "validate merged config")
	}
	return merged, nil
}

func (p *Provider) loadSourceConfig(
	ctx context.Context,
	sourceName string,
	configProvider provider.ConfigProvider,
	loadedConfigs *collectionlist.List[*config.Config],
	messages *collectionlist.List[string],
) {
	start := time.Now()
	cfg, err := configProvider.Load(ctx)
	if err != nil {
		p.handleSourceLoadError(ctx, sourceName, err, messages)
		return
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
}

func (p *Provider) handleSourceLoadError(ctx context.Context, sourceName string, err error, messages *collectionlist.List[string]) {
	messages.Add(fmt.Sprintf("config provider[%s] load failed: %v", sourceName, err))
	if p.logger != nil {
		p.logger.Error("config source load failed", "source", sourceName, "error", err)
	}
	p.publish(ctx, provider.ConfigSourceFailedEvent{
		Source: sourceName,
		Error:  err.Error(),
	})
}

func (p *Provider) reloadNow(ctx context.Context, sourceName string, onReload func(*runtime.CompiledSnapshot), onError func(error)) {
	if p.logger != nil {
		p.logger.Info("config source changed", "source", sourceName)
	}
	p.publish(ctx, provider.ConfigSourceChangedEvent{Source: sourceName})
	snapshot, fingerprint, err := p.loadSnapshot(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("reload merged snapshot failed", "source", sourceName, "error", err)
		}
		onError(err)
		return
	}
	if p.isSameFingerprint(fingerprint) {
		if p.logger != nil {
			p.logger.Info("merged snapshot unchanged", "source", sourceName, "fingerprint", fingerprint)
		}
		p.publish(ctx, provider.SnapshotUnchangedEvent{Source: sourceName, Fingerprint: fingerprint})
		return
	}
	p.storeFingerprint(fingerprint)
	p.publish(ctx, provider.SnapshotRecompiledEvent{
		SourceCount:  p.sourceCount(),
		RouteCount:   snapshot.Routes().Len(),
		ServiceCount: snapshot.ServicesView().Len(),
		Fingerprint:  fingerprint,
	})
	if p.logger != nil {
		p.logger.Info("merged snapshot reloaded", "source", sourceName, "built_at", snapshot.BuiltAt, "fingerprint", fingerprint)
	}
	onReload(snapshot)
}

func (p *Provider) storeFingerprint(fingerprint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastFingerprint = fingerprint
}

func (p *Provider) isSameFingerprint(fingerprint string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fingerprint != "" && fingerprint == p.lastFingerprint
}

func (p *Provider) publish(ctx context.Context, event provider.Event) {
	if p.bus == nil || event == nil {
		return
	}
	if err := p.bus.Publish(ctx, event); err != nil && p.logger != nil {
		p.logger.Error("publish provider event failed", "error", err)
	}
}

func (p *Provider) sourceCount() int {
	if p == nil || p.sources == nil {
		return 0
	}
	return p.sources.Len()
}

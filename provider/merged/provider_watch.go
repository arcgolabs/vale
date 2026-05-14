package merged

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
)

func (p *Provider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	if p.sources == nil || p.sources.Len() == 0 {
		return nil, errors.New("merged provider has no config providers")
	}
	watchCtx, cancel := context.WithCancel(ctx)
	changes := make(chan string, 32)
	sourceClosers := collectionlist.NewListWithCapacity[io.Closer](p.sources.Len())
	go p.runReloadLoop(watchCtx, changes, onReload, onError)

	if err := p.startSourceWatchers(ctx, watchCtx, changes, sourceClosers, onError); err != nil {
		cancel()
		p.closeSourceClosers(sourceClosers, onError)
		return nil, err
	}
	if sourceClosers.IsEmpty() {
		cancel()
		return nil, errors.New("merged provider failed to setup any watcher")
	}
	return provider.NewOnceCloser(func() {
		cancel()
		p.closeSourceClosers(sourceClosers, onError)
	}), nil
}

func (p *Provider) startSourceWatchers(
	ctx context.Context,
	watchCtx context.Context,
	changes chan<- string,
	sourceClosers *collectionlist.List[io.Closer],
	onError func(error),
) error {
	var setupErr error
	p.sources.Range(func(sourceName string, configProvider provider.ConfigProvider) bool {
		closer, err := p.watchSource(ctx, watchCtx, sourceName, configProvider, changes, onError)
		if err != nil {
			setupErr = err
			return false
		}
		sourceClosers.Add(closer)
		return true
	})
	return setupErr
}

func (p *Provider) watchSource(
	ctx, watchCtx context.Context,
	sourceName string,
	configProvider provider.ConfigProvider,
	changes chan<- string,
	onError func(error),
) (io.Closer, error) {
	if p.logger != nil {
		p.logger.Info("watching config source", "source", sourceName)
	}
	closer, err := configProvider.Watch(watchCtx, func() {
		notifySourceChanged(watchCtx, changes, sourceName)
	}, func(err error) {
		reportWatchError(onError, fmt.Errorf("config provider[%s] watch error: %w", sourceName, err))
	})
	if err != nil {
		p.handleWatchSetupError(ctx, sourceName, err, onError)
		return nil, fmt.Errorf("config provider[%s] watch setup failed: %w", sourceName, err)
	}
	if p.logger != nil {
		p.logger.Info("config source watcher ready", "source", sourceName)
	}
	return closer, nil
}

func notifySourceChanged(ctx context.Context, changes chan<- string, sourceName string) {
	select {
	case changes <- sourceName:
	case <-ctx.Done():
	}
}

func (p *Provider) handleWatchSetupError(ctx context.Context, sourceName string, err error, onError func(error)) {
	if p.logger != nil {
		p.logger.Error("config source watch setup failed", "source", sourceName, "error", err)
	}
	p.publish(ctx, provider.WatchSetupFailedEvent{
		Source: sourceName,
		Error:  err.Error(),
	})
	reportWatchError(onError, fmt.Errorf("config provider[%s] watch setup failed: %w", sourceName, err))
}

func (p *Provider) closeSourceClosers(sourceClosers *collectionlist.List[io.Closer], onError func(error)) {
	if sourceClosers == nil || sourceClosers.IsEmpty() {
		return
	}
	if err := provider.NewMultiCloser(sourceClosers).Close(); err != nil {
		reportWatchError(onError, fmt.Errorf("close merged provider watchers: %w", err))
	}
}

func (p *Provider) runReloadLoop(
	ctx context.Context,
	changes <-chan string,
	onReload func(*runtime.CompiledSnapshot),
	onError func(error),
) {
	pending := collectionset.NewSet[string]()
	timer := time.NewTimer(time.Hour)
	timerActive := stopTimer(timer)
	debounceStart := time.Time{}
	for {
		select {
		case <-ctx.Done():
			stopReloadTimer(timer, timerActive)
			return
		case sourceName := <-changes:
			pending.Add(sourceName)
			if !timerActive {
				timer.Reset(p.reloadDebounce)
				timerActive = true
				debounceStart = time.Now()
			}
		case <-timer.C:
			timerActive = false
			p.reloadPending(ctx, pending, debounceStart, onReload, onError)
			debounceStart = time.Time{}
		}
	}
}

func (p *Provider) reloadPending(
	ctx context.Context,
	pending *collectionset.Set[string],
	debounceStart time.Time,
	onReload func(*runtime.CompiledSnapshot),
	onError func(error),
) {
	sourceNames := collectionlist.NewListWithCapacity[string](pending.Len())
	pending.Range(func(sourceName string) bool {
		sourceNames.Add(sourceName)
		return true
	})
	sourceNames = provider.SortedStrings(sourceNames)
	pending.Clear()
	if sourceNames.IsEmpty() {
		return
	}
	debounceTime := time.Since(debounceStart)
	p.publish(ctx, provider.ConfigSourceDebouncedEvent{
		Source:       sourceNames.Join(","),
		DebounceTime: debounceTime,
		SourceCount:  sourceNames.Len(),
	})
	p.reloadNow(ctx, sourceNames.Join(","), onReload, onError)
}

func stopTimer(timer *time.Timer) bool {
	if !timer.Stop() {
		<-timer.C
	}
	return false
}

func stopReloadTimer(timer *time.Timer, timerActive bool) {
	if timerActive {
		timer.Stop()
	}
}

func reportWatchError(onError func(error), err error) {
	if onError != nil {
		onError(err)
	}
}

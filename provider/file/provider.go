package file

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/arcgolabs/vela/compiler"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/runtime"
	"github.com/fsnotify/fsnotify"
)

type Provider struct {
	configPath string
	logger     *slog.Logger
}

func New(configPath string, logger *slog.Logger) *Provider {
	if logger == nil {
		logger = slog.Default()
	}
	return &Provider{
		configPath: configPath,
		logger:     logger,
	}
}

func (p *Provider) Load(_ context.Context) (*runtime.CompiledSnapshot, error) {
	cfg, err := config.Load(p.configPath)
	if err != nil {
		return nil, err
	}
	return compiler.Compile(cfg)
}

func (p *Provider) Watch(_ context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(p.configPath)
	base := filepath.Base(p.configPath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, err
	}

	go func() {
		var lastReload time.Time
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != base {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
					continue
				}
				if time.Since(lastReload) < 300*time.Millisecond {
					continue
				}
				lastReload = time.Now()

				snapshot, loadErr := p.Load(context.Background())
				if loadErr != nil {
					onError(loadErr)
					continue
				}
				onReload(snapshot)
				p.logger.Info("snapshot reloaded", "built_at", snapshot.BuiltAt)
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				onError(watchErr)
			}
		}
	}()
	return watcher, nil
}

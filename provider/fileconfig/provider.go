package fileconfig

import (
	"context"
	"io"
	"path/filepath"
	"time"

	"github.com/arcgolabs/gateway/config"
	"github.com/fsnotify/fsnotify"
)

type Provider struct {
	path string
}

func New(path string) *Provider {
	return &Provider{path: path}
}

func (p *Provider) Name() string {
	return p.path
}

func (p *Provider) Load(_ context.Context) (*config.Config, error) {
	return config.Load(p.path)
}

func (p *Provider) Watch(_ context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(p.path)
	base := filepath.Base(p.path)
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
				onReload()
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

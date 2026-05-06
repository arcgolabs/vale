package fileconfig

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/provider"
	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/hcl/v2/hclsimple"
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
	return Load(p.path)
}

func (p *Provider) Watch(_ context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	return WatchPath(p.path, onReload, onError)
}

func WatchPath(path string, onChange func(), onError func(error)) (io.Closer, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
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
				onChange()
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

func Load(path string) (*config.Config, error) {
	var cfg config.Config
	if err := hclsimple.DecodeFile(path, nil, &cfg); err != nil {
		return nil, err
	}
	if err := config.Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func WithConfigPath(path string) gateway.Option {
	return func(cfg *gateway.Config) error {
		if path == "" {
			return fmt.Errorf("config path cannot be empty")
		}
		cfg.ConfigSource = []provider.ConfigProvider{New(path)}
		cfg.Provider = nil
		return nil
	}
}

func WithConfigFiles(paths ...string) gateway.Option {
	return func(cfg *gateway.Config) error {
		if len(paths) == 0 {
			return fmt.Errorf("config files cannot be empty")
		}
		providers := make([]provider.ConfigProvider, 0, len(paths))
		for _, path := range paths {
			if path == "" {
				return fmt.Errorf("config file path cannot be empty")
			}
			providers = append(providers, New(path))
		}
		cfg.ConfigSource = providers
		cfg.Provider = nil
		return nil
	}
}

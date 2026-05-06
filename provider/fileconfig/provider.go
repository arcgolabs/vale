package fileconfig

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/provider"
	"github.com/fsnotify/fsnotify"
	"github.com/hashicorp/hcl/v2/hclsimple"
)

type Provider struct {
	path   string
	logger *slog.Logger
}

func New(path string) *Provider {
	return &Provider{path: path}
}

func NewWithLogger(path string, logger *slog.Logger) *Provider {
	return &Provider{
		path:   path,
		logger: logger,
	}
}

func (p *Provider) SetLogger(logger *slog.Logger) {
	p.logger = logger
}

func (p *Provider) Name() string {
	return p.path
}

func (p *Provider) Load(_ context.Context) (*config.Config, error) {
	if p.logger != nil {
		p.logger.Info("loading config file", "path", p.path)
	}
	cfg, err := Load(p.path)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("config file load failed", "path", p.path, "error", err)
		}
		return nil, err
	}
	if p.logger != nil {
		p.logger.Info("config file loaded",
			"path", p.path,
			"entrypoints", len(cfg.Entrypoints),
			"services", len(cfg.Services),
			"routes", len(cfg.Routes),
		)
	}
	return cfg, nil
}

func (p *Provider) Watch(_ context.Context, onReload func(), onError func(error)) (io.Closer, error) {
	return WatchPathWithLogger(p.path, p.logger, onReload, onError)
}

func WatchPath(path string, onChange func(), onError func(error)) (io.Closer, error) {
	return WatchPathWithLogger(path, nil, onChange, onError)
}

func WatchPathWithLogger(path string, logger *slog.Logger, onChange func(), onError func(error)) (io.Closer, error) {
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
	if logger != nil {
		logger.Info("watching config file", "path", path, "dir", dir)
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
				if logger != nil {
					logger.Info("config file changed", "path", path, "op", event.Op.String())
				}
				onChange()
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}
				if logger != nil {
					logger.Error("config file watch error", "path", path, "error", watchErr)
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

// Package fileconfig provides HCL file config loading and watching.
package fileconfig

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
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
	watcher, err := newConfigFileWatcher(path, logger)
	if err != nil {
		return nil, err
	}
	go fileWatch{
		path:     path,
		base:     filepath.Base(path),
		watcher:  watcher,
		logger:   logger,
		onChange: onChange,
		onError:  onError,
	}.run()
	return watcher, nil
}

type fileWatch struct {
	path     string
	base     string
	watcher  *fsnotify.Watcher
	logger   *slog.Logger
	onChange func()
	onError  func(error)
}

func newConfigFileWatcher(path string, logger *slog.Logger) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create config file watcher: %w", err)
	}

	dir := filepath.Dir(path)
	if err := watcher.Add(dir); err != nil {
		closeErr := watcher.Close()
		return nil, errors.Join(
			fmt.Errorf("watch config directory %q: %w", dir, err),
			closeErr,
		)
	}
	if logger != nil {
		logger.Info("watching config file", "path", path, "dir", dir)
	}
	return watcher, nil
}

func (w fileWatch) run() {
	var lastReload time.Time
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event, &lastReload)
		case watchErr, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.handleError(watchErr)
		}
	}
}

func (w fileWatch) handleEvent(event fsnotify.Event, lastReload *time.Time) {
	if !w.shouldReload(event, *lastReload) {
		return
	}
	*lastReload = time.Now()
	if w.logger != nil {
		w.logger.Info("config file changed", "path", w.path, "op", event.Op.String())
	}
	if w.onChange != nil {
		w.onChange()
	}
}

func (w fileWatch) shouldReload(event fsnotify.Event, lastReload time.Time) bool {
	if filepath.Base(event.Name) != w.base {
		return false
	}
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
		return false
	}
	return time.Since(lastReload) >= 300*time.Millisecond
}

func (w fileWatch) handleError(watchErr error) {
	if w.logger != nil {
		w.logger.Error("config file watch error", "path", w.path, "error", watchErr)
	}
	if w.onError != nil {
		w.onError(watchErr)
	}
}

func Load(path string) (*config.Config, error) {
	var cfg config.Config
	if err := hclsimple.DecodeFile(path, nil, &cfg); err != nil {
		return nil, fmt.Errorf("decode config file %q: %w", path, err)
	}
	if err := config.Validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config file %q: %w", path, err)
	}
	return &cfg, nil
}

func WithConfigPath(path string) gateway.Option {
	return func(cfg *gateway.Config) error {
		if path == "" {
			return errors.New("config path cannot be empty")
		}
		cfg.ConfigSource = collectionlist.NewList[provider.ConfigProvider](New(path))
		cfg.Provider = nil
		return nil
	}
}

func WithConfigFiles(paths ...string) gateway.Option {
	return WithConfigFileList(collectionlist.NewList(paths...))
}

func WithConfigFileList(paths *collectionlist.List[string]) gateway.Option {
	return func(cfg *gateway.Config) error {
		if paths == nil || paths.IsEmpty() {
			return errors.New("config files cannot be empty")
		}
		if paths.AnyMatch(func(_ int, path string) bool {
			return path == ""
		}) {
			return errors.New("config file path cannot be empty")
		}
		providers := collectionlist.MapList(
			paths,
			func(_ int, path string) provider.ConfigProvider {
				return New(path)
			},
		)
		cfg.ConfigSource = providers
		cfg.Provider = nil
		return nil
	}
}

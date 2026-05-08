// Package file provides a snapshot provider backed by an HCL config file.
package file

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/arcgolabs/vale/compiler"
	fileconfig "github.com/arcgolabs/vale/provider/fileconfig"
	"github.com/arcgolabs/vale/runtime"
)

type Provider struct {
	configPath string
	logger     *slog.Logger
}

func New(configPath string, logger *slog.Logger) *Provider {
	return &Provider{
		configPath: configPath,
		logger:     logger,
	}
}

func (p *Provider) SetLogger(logger *slog.Logger) {
	p.logger = logger
}

func (p *Provider) Load(_ context.Context) (*runtime.CompiledSnapshot, error) {
	if p.logger != nil {
		p.logger.Info("loading snapshot from config file", "path", p.configPath)
	}
	cfg, err := fileconfig.Load(p.configPath)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("snapshot config load failed", "path", p.configPath, "error", err)
		}
		return nil, fmt.Errorf("load snapshot config file %q: %w", p.configPath, err)
	}
	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("snapshot compile failed", "path", p.configPath, "error", err)
		}
		return nil, fmt.Errorf("compile snapshot config file %q: %w", p.configPath, err)
	}
	if p.logger != nil {
		p.logger.Info("snapshot loaded",
			"path", p.configPath,
			"built_at", snapshot.BuiltAt,
			"entrypoints", snapshot.Entrypoints.Len(),
			"services", snapshot.Services.Len(),
			"routes", snapshot.Routes().Len(),
		)
	}
	return snapshot, nil
}

func (p *Provider) Watch(ctx context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	closer, err := fileconfig.WatchPathWithLogger(p.configPath, p.logger, func() {
		snapshot, loadErr := p.Load(ctx)
		if loadErr != nil {
			onError(loadErr)
			return
		}
		onReload(snapshot)
		if p.logger != nil {
			p.logger.Info("snapshot reloaded", "built_at", snapshot.BuiltAt)
		}
	}, onError)
	if err != nil {
		return nil, fmt.Errorf("watch snapshot config file %q: %w", p.configPath, err)
	}
	return closer, nil
}

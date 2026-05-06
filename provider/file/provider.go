package file

import (
	"context"
	"io"
	"log/slog"

	"github.com/arcgolabs/vela/compiler"
	fileconfig "github.com/arcgolabs/vela/provider/fileconfig"
	"github.com/arcgolabs/vela/runtime"
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
		return nil, err
	}
	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("snapshot compile failed", "path", p.configPath, "error", err)
		}
		return nil, err
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

func (p *Provider) Watch(_ context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	return fileconfig.WatchPathWithLogger(p.configPath, p.logger, func() {
		snapshot, loadErr := p.Load(context.Background())
		if loadErr != nil {
			onError(loadErr)
			return
		}
		onReload(snapshot)
		p.logger.Info("snapshot reloaded", "built_at", snapshot.BuiltAt)
	}, onError)
}

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
	if logger == nil {
		logger = slog.Default()
	}
	return &Provider{
		configPath: configPath,
		logger:     logger,
	}
}

func (p *Provider) Load(_ context.Context) (*runtime.CompiledSnapshot, error) {
	cfg, err := fileconfig.Load(p.configPath)
	if err != nil {
		return nil, err
	}
	return compiler.Compile(cfg)
}

func (p *Provider) Watch(_ context.Context, onReload func(*runtime.CompiledSnapshot), onError func(error)) (io.Closer, error) {
	return fileconfig.WatchPath(p.configPath, func() {
		snapshot, loadErr := p.Load(context.Background())
		if loadErr != nil {
			onError(loadErr)
			return
		}
		onReload(snapshot)
		p.logger.Info("snapshot reloaded", "built_at", snapshot.BuiltAt)
	}, onError)
}

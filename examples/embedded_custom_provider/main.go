// Custom config provider extension: register a provider factory, instantiate it
// from a ProviderSpec, and let Vale merge/compile it through the normal gateway
// path.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
)

const catalogProviderType = "catalog"

type catalogApp struct {
	PathPrefix string
	URL        string
}

type catalogProvider struct {
	name string
	apps *mapping.Map[string, catalogApp]
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(context.Background(), logger); err != nil {
		logger.Error("embedded gateway failed", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backendAddr, stopBackend, err := startBackend(parent, logger)
	if err != nil {
		return err
	}
	defer stopBackend()

	registry := vale.NewRegistry()
	if registerErr := registry.RegisterConfigProvider(catalogProviderType, newCatalogProvider); registerErr != nil {
		return fmt.Errorf("register catalog provider: %w", registerErr)
	}

	catalog, err := registry.CreateConfigProvider(ctx, vale.NewProviderSpec(catalogProviderType).
		WithName("in-process-catalog").
		WithSetting("api_url", "http://"+backendAddr))
	if err != nil {
		return fmt.Errorf("create catalog provider: %w", err)
	}

	embeddedGateway, err := vale.New(
		vale.WithLogger(logger),
		vale.WithRegistry(registry),
		vale.WithConfigSourceProviders(catalog),
		vale.WithWatch(false),
	)
	if err != nil {
		return fmt.Errorf("create embedded gateway: %w", err)
	}
	if startErr := embeddedGateway.Start(ctx); startErr != nil {
		return fmt.Errorf("start embedded gateway: %w", startErr)
	}
	logger.Info("embedded gateway started", "gateway", "http://127.0.0.1:8080/catalog", "admin", "http://127.0.0.1:19090")

	<-ctx.Done()
	if err := stopGateway(parent, logger, embeddedGateway); err != nil {
		return err
	}
	return nil
}

func newCatalogProvider(_ context.Context, spec vale.ProviderSpec) (provider.ConfigProvider, error) {
	apiURL, ok := spec.Setting("api_url")
	if !ok || apiURL == "" {
		return nil, errors.New("catalog provider requires api_url setting")
	}
	apps := mapping.NewMap[string, catalogApp]()
	apps.Set("catalog-api", catalogApp{
		PathPrefix: "/catalog",
		URL:        apiURL,
	})
	return &catalogProvider{name: spec.Name, apps: apps}, nil
}

func (p *catalogProvider) Name() string {
	return p.name
}

func (p *catalogProvider) Load(context.Context) (*config.Config, error) {
	builder := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		Admin(":19090").
		Observability(true, true)
	p.apps.Range(func(name string, app catalogApp) bool {
		builder.Service(name, app.URL).
			RouteTo(name+"-route", "web", name, vale.RoutePathPrefix(app.PathPrefix))
		return true
	})
	cfg, err := builder.BuildValidated()
	if err != nil {
		return nil, fmt.Errorf("build catalog config: %w", err)
	}
	return cfg, nil
}

func (*catalogProvider) Watch(context.Context, func(), func(error)) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

func startBackend(ctx context.Context, logger *slog.Logger) (string, func(), error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen backend: %w", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, writeErr := w.Write([]byte("catalog backend\n")); writeErr != nil {
				logger.Warn("write backend response failed", "error", writeErr)
			}
		}),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("backend crashed", "error", serveErr)
		}
	}()
	return listener.Addr().String(), func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("backend shutdown failed", "error", err)
		}
	}, nil
}

func stopGateway(ctx context.Context, logger *slog.Logger, gateway *vale.Gateway) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := gateway.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("stop embedded gateway: %w", err)
	}
	logger.Info("embedded gateway stopped")
	return nil
}

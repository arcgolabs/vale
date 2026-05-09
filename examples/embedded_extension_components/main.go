// Extension component registry: certificate storage, observability, and a
// cluster factory can be composed through Vale's public Registry without a
// runtime plugin system.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/certstore"
)

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
	if registerErr := registerComponents(registry); registerErr != nil {
		return registerErr
	}

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		Entrypoint("websecure", ":8443", vale.EntrypointACME("ops@example.com", "", "example.com")).
		Service("api", "http://"+backendAddr).
		RouteTo("api-http", "web", "api", vale.RoutePathPrefix("/")).
		RouteTo("api-https", "websecure", "api", vale.RoutePathPrefix("/")).
		Admin(":19090").
		Observability(true, true).
		Build()

	embeddedGateway, err := vale.New(
		vale.WithLogger(logger),
		vale.WithRegistry(registry),
		vale.WithCertificateStorageFromRegistry(parent, registry, "memory"),
		vale.WithObservabilityFromRegistry(registry, "noop"),
		vale.WithClusterFromRegistry(registry, "single-node"),
		vale.WithStaticConfig(cfg),
	)
	if err != nil {
		return fmt.Errorf("create embedded gateway: %w", err)
	}
	if startErr := embeddedGateway.Start(ctx); startErr != nil {
		return fmt.Errorf("start embedded gateway: %w", startErr)
	}
	logger.Info("embedded gateway started", "http", "http://127.0.0.1:8080", "https", "https://127.0.0.1:8443", "admin", "http://127.0.0.1:19090")

	<-ctx.Done()
	if err := stopGateway(parent, logger, embeddedGateway); err != nil {
		return err
	}
	return nil
}

func registerComponents(registry *vale.Registry) error {
	if err := registry.RegisterCertificateStorage("memory", func(context.Context) (certstore.Storage, error) {
		return certstore.NewProjection(), nil
	}); err != nil {
		return fmt.Errorf("register certificate storage: %w", err)
	}
	if err := registry.RegisterObservabilityFactory("noop", func(*slog.Logger) (observabilityx.Observability, error) {
		return observabilityx.Nop(), nil
	}); err != nil {
		return fmt.Errorf("register observability: %w", err)
	}
	if err := registry.RegisterClusterFactory("single-node", func(*slog.Logger) (vale.Cluster, error) {
		return singleNodeCluster{}, nil
	}); err != nil {
		return fmt.Errorf("register cluster factory: %w", err)
	}
	return nil
}

type singleNodeCluster struct{}

func (singleNodeCluster) IsLeader() bool {
	return true
}

func (singleNodeCluster) Apply([]byte, time.Duration) error {
	return nil
}

func (singleNodeCluster) Status() *mapping.Map[string, any] {
	status := mapping.NewMap[string, any]()
	status.Set("mode", "single-node-example")
	status.Set("leader", true)
	return status
}

func (singleNodeCluster) Peers() (*collectionlist.List[*vale.ClusterPeer], error) {
	peer := mapping.NewMap[string, string]()
	peer.Set("id", "node-1")
	peer.Set("address", "in-process")
	return collectionlist.NewList(peer), nil
}

func (singleNodeCluster) AddVoter(string, string, time.Duration) error {
	return nil
}

func (singleNodeCluster) RemoveServer(string, time.Duration) error {
	return nil
}

func (singleNodeCluster) Shutdown() error {
	return nil
}

func startBackend(ctx context.Context, logger *slog.Logger) (string, func(), error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen backend: %w", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, writeErr := w.Write([]byte("component example backend\n")); writeErr != nil {
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

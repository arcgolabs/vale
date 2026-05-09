// Custom middleware extension: register a typed middleware factory and use it
// from normal Vale config. This is compile-time library composition, not a
// runtime plugin loader.
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

	"github.com/arcgolabs/vale"
)

const tenantMiddlewareType = "tenant_header"

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
	if registerErr := registry.RegisterMiddleware(tenantMiddlewareType, tenantHeaderMiddleware("embedded")); registerErr != nil {
		return fmt.Errorf("register tenant middleware: %w", registerErr)
	}

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		MiddlewareNamed("tenant", vale.MiddlewareType(tenantMiddlewareType)).
		Service("api", "http://"+backendAddr).
		RouteTo("api", "web", "api", vale.RoutePathPrefix("/"), vale.RouteMiddlewares("tenant")).
		Admin(":19090").
		Observability(true, true).
		Build()

	embeddedGateway, err := vale.New(
		vale.WithLogger(logger),
		vale.WithRegistry(registry),
		vale.WithStaticConfig(cfg),
	)
	if err != nil {
		return fmt.Errorf("create embedded gateway: %w", err)
	}

	if err := embeddedGateway.Start(ctx); err != nil {
		return fmt.Errorf("start embedded gateway: %w", err)
	}
	logger.Info("embedded gateway started", "gateway", "http://127.0.0.1:8080", "admin", "http://127.0.0.1:19090")

	<-ctx.Done()
	if err := stopGateway(parent, logger, embeddedGateway); err != nil {
		return err
	}
	return nil
}

func tenantHeaderMiddleware(tenant string) vale.MiddlewareFactory {
	return func(next http.Handler, _ vale.RuntimeMiddleware) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Header.Set("X-Tenant", tenant)
			w.Header().Set("X-Tenant", tenant)
			next.ServeHTTP(w, r)
		})
	}
}

func startBackend(ctx context.Context, logger *slog.Logger) (string, func(), error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen backend: %w", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if _, writeErr := w.Write([]byte("tenant=embedded\n")); writeErr != nil {
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

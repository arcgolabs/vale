// GatewayBuilder components: compose registry extensions and constructor options
// without putting process-specific wiring into the root Vale package.
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

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		MiddlewareNamed("request-id", vale.MiddlewareType("request_id")).
		Service("api", "http://"+backendAddr).
		RouteTo("api", "web", "api",
			vale.RoutePathPrefix("/"),
			vale.RouteMiddlewares("request-id"),
		).
		Admin(":19090").
		Observability(false, false).
		Build()

	embeddedGateway, err := vale.NewGatewayBuilder(
		requestIDComponent(),
		vale.GatewayOptions(
			vale.WithLogger(logger),
			vale.WithStaticConfig(cfg),
		),
	).Build()
	if err != nil {
		return fmt.Errorf("create embedded gateway: %w", err)
	}
	if startErr := embeddedGateway.Start(ctx); startErr != nil {
		return fmt.Errorf("start embedded gateway: %w", startErr)
	}
	logger.Info("embedded gateway started", "http", "http://127.0.0.1:8080", "admin", "http://127.0.0.1:19090")

	<-ctx.Done()
	return stopGateway(parent, logger, embeddedGateway)
}

func requestIDComponent() vale.GatewayComponent {
	return vale.GatewayComponentFunc(func(builder *vale.GatewayBuilder) error {
		return builder.Registry().RegisterMiddleware("request_id", requestIDMiddleware)
	})
}

func requestIDMiddleware(next http.Handler, _ vale.RuntimeMiddleware) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Vale-Request-ID", r.Header.Get("X-Request-ID"))
		next.ServeHTTP(w, r)
	})
}

func startBackend(ctx context.Context, logger *slog.Logger) (string, func(), error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen backend: %w", err)
	}
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
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
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil {
			logger.Error("backend shutdown failed", "error", shutdownErr)
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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/arcgolabs/vale"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func startBackend(ctx context.Context, logger *slog.Logger) (string, func(), error) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte("jwt authenticated\n")); err != nil {
			logger.Warn("write backend response failed", "error", err)
		}
	})
	return startHTTPServer(ctx, logger, "backend", handler)
}

func startHTTPServer(ctx context.Context, logger *slog.Logger, name string, handler http.Handler) (string, func(), error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("listen %s: %w", name, err)
	}
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			logger.Error("http server crashed", "name", name, "error", serveErr)
		}
	}()
	return listener.Addr().String(), func() {
		shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("http server shutdown failed", "name", name, "error", err)
		}
	}, nil
}

func stopGateway(ctx context.Context, logger *slog.Logger, gateway *vale.Gateway) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()
	if err := gateway.Stop(shutdownCtx); err != nil {
		return fmt.Errorf("stop embedded gateway: %w", err)
	}
	logger.Info("embedded gateway stopped")
	return nil
}

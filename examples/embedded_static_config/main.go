// Static in-process config: validated *config.Config, shared eventx bus, and vale.WithStaticConfig.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vale"
	providerevents "github.com/arcgolabs/vale/provider"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	bus := eventx.New()

	_, err := eventx.Subscribe[providerevents.SnapshotRecompiledEvent](bus, func(_ context.Context, event providerevents.SnapshotRecompiledEvent) error {
		logger.Info("snapshot recompiled", "sources", event.SourceCount, "routes", event.RouteCount, "services", event.ServiceCount)
		return nil
	})
	if err != nil {
		logger.Error("subscribe recompiled event failed", "error", err)
		os.Exit(1)
	}
	_, err = eventx.Subscribe[providerevents.ConfigSourceFailedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceFailedEvent) error {
		logger.Error("config source failed", "source", event.Source, "error", event.Error)
		return nil
	})
	if err != nil {
		logger.Error("subscribe config source failed event failed", "error", err)
		os.Exit(1)
	}

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		Service("echo", "http://127.0.0.1:8081").
		RouteTo("echo-route", "web", "echo", vale.RoutePathPrefix("/")).
		Admin(":19090").
		Observability(true, true).
		Health("5s", "2s").
		Build()

	embeddedGateway, err := vale.New(
		vale.WithLogger(logger),
		vale.WithEventBus(bus),
		vale.WithStaticConfig(cfg),
	)
	if err != nil {
		logger.Error("create embedded gateway failed", "error", err)
		os.Exit(1)
	}

	if err := embeddedGateway.Start(context.Background()); err != nil {
		logger.Error("start embedded gateway failed", "error", err)
		os.Exit(1)
	}
	logger.Info("embedded gateway started")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := embeddedGateway.Stop(ctx); err != nil {
		cancel()
		logger.Error("stop embedded gateway failed", "error", err)
		os.Exit(1)
	}
	cancel()
	logger.Info("embedded gateway stopped")
}

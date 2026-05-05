// Static in-process config: validated *config.Config, shared eventx bus, and vela.WithStaticConfig.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vela"
	"github.com/arcgolabs/vela/config"
	providerevents "github.com/arcgolabs/vela/provider"
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

	cfg := &config.Config{
		ProxyEngine: "stdlib",
		Entrypoints: []config.Entrypoint{
			{Name: "web", Address: ":8080"},
		},
		Services: []config.Service{
			{
				Name:     "echo",
				Strategy: "round_robin",
				Endpoints: []config.Endpoint{
					{URL: "http://127.0.0.1:8081", Weight: 1},
				},
			},
		},
		Routes: []config.Route{
			{
				Name:       "echo-route",
				Entrypoint: "web",
				Service:    "echo",
				PathPrefix: "/",
			},
		},
		Admin: &config.Admin{Address: ":19090"},
		Observability: &config.Observability{
			AccessLog: true,
			Metrics:   true,
		},
		Health: &config.Health{
			Interval: "5s",
			Timeout:  "2s",
		},
	}

	embeddedGateway, err := vela.New(
		vela.WithLogger(logger),
		vela.WithEventBus(bus),
		vela.WithStaticConfig(cfg),
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
	defer cancel()
	_ = embeddedGateway.Stop(ctx)
	logger.Info("embedded gateway stopped")
}

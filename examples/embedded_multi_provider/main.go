// Memory-backed docker and k8s providers merged into one gateway (route /a vs /b).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vale"
	providerevents "github.com/arcgolabs/vale/provider"
	providerdocker "github.com/arcgolabs/vale/provider/docker"
	providerk8s "github.com/arcgolabs/vale/provider/k8s"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	bus := eventx.New()

	_, err := eventx.Subscribe[providerevents.ConfigSourceLoadedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceLoadedEvent) error {
		logger.Info("config source loaded", "source", event.Source, "duration", event.Duration, "size", event.ConfigSize)
		return nil
	})
	if err != nil {
		logger.Error("subscribe config source loaded event failed", "error", err)
		os.Exit(1)
	}

	dockerSource := providerdocker.NewMemorySource(
		providerdocker.Container{
			Name:    "app-a",
			Address: "127.0.0.1",
			Port:    8081,
			Labels: mapping.NewMapFrom(map[string]string{
				"vale.enable":          "true",
				"vale.service":         "svc-app-a",
				"vale.route":           "route-app-a",
				"vale.entrypoint":      "web",
				"vale.rule.pathprefix": "/a",
			}),
		},
	)
	k8sSource := providerk8s.NewMemorySource(
		collectionlist.NewList(
			providerk8s.HTTPRoute{
				Name:       "route-app-b",
				Entrypoint: "web",
				PathPrefix: "/b",
				Service:    "svc-app-b",
			},
		),
		collectionlist.NewList(
			providerk8s.ServiceEndpoint{
				Service: "svc-app-b",
				URL:     "http://127.0.0.1:8082",
				Weight:  1,
			},
		),
	)

	dockerProvider := providerdocker.New("docker-mem", dockerSource, providerdocker.DefaultOptions())
	k8sProvider := providerk8s.New("k8s-mem", k8sSource, providerk8s.DefaultOptions())

	embeddedGateway, err := vale.New(
		vale.WithLogger(logger),
		vale.WithEventBus(bus),
		vale.WithConfigSourceProviders(dockerProvider, k8sProvider),
		vale.WithWatch(true),
	)
	if err != nil {
		logger.Error("create embedded gateway failed", "error", err)
		os.Exit(1)
	}

	if err := embeddedGateway.Start(context.Background()); err != nil {
		logger.Error("start embedded gateway failed", "error", err)
		os.Exit(1)
	}
	logger.Info("embedded gateway started", "hint", "route /a from docker, route /b from k8s")

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

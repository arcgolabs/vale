// Memory-backed docker and k8s providers merged into one gateway (route /a vs /b).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vela/gateway"
	providerevents "github.com/arcgolabs/vela/provider"
	providerdocker "github.com/arcgolabs/vela/provider/docker"
	providerk8s "github.com/arcgolabs/vela/provider/k8s"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	bus := eventx.New()

	_, _ = eventx.Subscribe[providerevents.ConfigSourceLoadedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceLoadedEvent) error {
		logger.Info("config source loaded", "source", event.Source, "duration", event.Duration, "size", event.ConfigSize)
		return nil
	})

	dockerSource := providerdocker.NewMemorySource(
		providerdocker.Container{
			Name:    "app-a",
			Address: "127.0.0.1",
			Port:    8081,
			Labels: map[string]string{
				"vela.enable":          "true",
				"vela.service":         "svc-app-a",
				"vela.route":           "route-app-a",
				"vela.entrypoint":      "web",
				"vela.rule.pathprefix": "/a",
			},
		},
	)
	k8sSource := providerk8s.NewMemorySource(
		[]providerk8s.HTTPRoute{
			{
				Name:       "route-app-b",
				Entrypoint: "web",
				PathPrefix: "/b",
				Service:    "svc-app-b",
			},
		},
		[]providerk8s.ServiceEndpoint{
			{
				Service: "svc-app-b",
				URL:     "http://127.0.0.1:8082",
				Weight:  1,
			},
		},
	)

	dockerProvider := providerdocker.New("docker-mem", dockerSource, providerdocker.DefaultOptions())
	k8sProvider := providerk8s.New("k8s-mem", k8sSource, providerk8s.DefaultOptions())

	embeddedGateway, err := gateway.New(
		gateway.WithLogger(logger),
		gateway.WithEventBus(bus),
		gateway.WithDockerProvider(dockerProvider),
		gateway.WithK8sProvider(k8sProvider),
		gateway.WithWatch(true),
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
	defer cancel()
	_ = embeddedGateway.Stop(ctx)
	logger.Info("embedded gateway stopped")
}

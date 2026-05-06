package prometheus

import (
	"log/slog"

	observabilityprom "github.com/arcgolabs/observabilityx/prometheus"
	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/runtime"
)

func New(enabled bool, logger *slog.Logger) runtime.MetricsRecorder {
	adapter := observabilityprom.New(
		observabilityprom.WithLogger(logger),
		observabilityprom.WithNamespace("vela"),
	)
	return runtime.NewObservabilityMetrics(enabled, adapter, adapter.Handler())
}

func WithMetrics() gateway.Option {
	return gateway.WithMetricsFactory(New)
}

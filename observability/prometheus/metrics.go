// Package prometheus provides a Prometheus metrics adapter for Vale.
package prometheus

import (
	"log/slog"

	"github.com/arcgolabs/observabilityx"
	observabilityprom "github.com/arcgolabs/observabilityx/prometheus"
	"github.com/arcgolabs/vale/gateway"
	"github.com/arcgolabs/vale/runtime"
)

func NewObservability(logger *slog.Logger) observabilityx.Observability {
	return observabilityprom.New(
		observabilityprom.WithLogger(logger),
		observabilityprom.WithNamespace("vale"),
	)
}

func New(enabled bool, logger *slog.Logger) runtime.MetricsRecorder {
	return NewWithObservability(enabled, NewObservability(logger))
}

func NewWithObservability(enabled bool, obs observabilityx.Observability) runtime.MetricsRecorder {
	return runtime.NewObservabilityMetrics(enabled, obs, nil)
}

func NewFactory(obs observabilityx.Observability) gateway.MetricsFactory {
	return func(enabled bool, _ *slog.Logger) runtime.MetricsRecorder {
		return NewWithObservability(enabled, obs)
	}
}

func WithMetrics() gateway.Option {
	return gateway.WithMetricsFactory(New)
}

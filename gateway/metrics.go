package gateway

import (
	"log/slog"

	"github.com/arcgolabs/vale/runtime"
)

type MetricsFactory func(enabled bool, logger *slog.Logger) runtime.MetricsRecorder

func (g *Gateway) buildMetrics(enabled bool) runtime.MetricsRecorder {
	if g.config.Metrics == nil {
		return runtime.NewObservabilityMetrics(enabled, g.config.Observability, nil)
	}
	return g.config.Metrics(enabled, g.logger)
}

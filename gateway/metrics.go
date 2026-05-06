package gateway

import "github.com/arcgolabs/vela/runtime"

type MetricsFactory func(enabled bool) runtime.MetricsRecorder

func (g *Gateway) buildMetrics(enabled bool) runtime.MetricsRecorder {
	if g.config.Metrics == nil {
		return runtime.NewNoopMetrics()
	}
	return g.config.Metrics(enabled)
}

package gateway

import "github.com/arcgolabs/vela/provider"

// Events returns the event bus configured with WithEventBus or the internal instance.
func (g *Gateway) Events() provider.EventBus {
	return g.events
}

// Status returns a coarse snapshot-only map (started flag, counts, and cluster status when enabled).
func (g *Gateway) Status() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	status := map[string]any{
		"started": g.started,
	}
	if g.runtime != nil && g.runtime.Snapshot() != nil {
		snapshot := g.runtime.Snapshot()
		status["built_at"] = snapshot.BuiltAt
		status["entrypoints"] = snapshot.Entrypoints.Len()
		status["services"] = snapshot.Services.Len()
		status["routes"] = snapshot.Routes().Len()
	}
	if g.cluster != nil {
		status["cluster"] = g.cluster.Status()
	} else {
		status["cluster"] = map[string]any{"enabled": false}
	}
	return status
}

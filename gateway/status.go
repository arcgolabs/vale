package gateway

import (
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/provider"
)

// Events returns the event bus configured with WithEventBus or the internal instance.
func (g *Gateway) Events() provider.EventBus {
	return g.events
}

// Status returns a coarse snapshot-only map (started flag, counts, and cluster status when enabled).
func (g *Gateway) Status() *mapping.Map[string, any] {
	g.mu.Lock()
	defer g.mu.Unlock()
	status := mapping.NewMap[string, any]()
	status.Set("started", g.started)
	if g.runtime != nil && g.runtime.Snapshot() != nil {
		snapshot := g.runtime.Snapshot()
		status.Set("built_at", snapshot.BuiltAt)
		status.Set("entrypoints", snapshot.Entrypoints.Len())
		status.Set("services", snapshot.Services.Len())
		status.Set("routes", snapshot.Routes().Len())
	}
	if g.cluster != nil {
		status.Set("cluster", g.cluster.Status())
	} else {
		status.Set("cluster", disabledClusterStatus())
	}
	return status
}

func disabledClusterStatus() *mapping.Map[string, any] {
	status := mapping.NewMap[string, any]()
	status.Set("enabled", false)
	return status
}

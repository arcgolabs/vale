package gateway

import (
	"encoding/json"
	"time"

	"github.com/arcgolabs/vela/runtime"
)

func (g *Gateway) publishClusterUpdate(snapshot *runtime.CompiledSnapshot) {
	if g.cluster == nil || !g.cluster.IsLeader() || snapshot == nil {
		return
	}
	payload := map[string]any{
		"type": "route_sync",
		"snapshot": map[string]any{
			"built_at":     snapshot.BuiltAt.UTC().Format(time.RFC3339Nano),
			"services":     snapshot.Services.Len(),
			"routes":       snapshot.Routes().Len(),
			"proxy_engine": snapshot.ProxyEngine,
		},
		"routes": adminRoutesView(snapshot, runtime.RouteFilter{}),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		g.logger.Error("raft payload marshal failed", "error", err)
		return
	}
	if err := g.cluster.Apply(data, 2*time.Second); err != nil {
		g.logger.Error("raft apply failed", "error", err)
	}
}

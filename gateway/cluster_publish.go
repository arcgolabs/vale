package gateway

import (
	"encoding/json"
	"time"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/runtime"
)

func (g *Gateway) publishClusterUpdate(snapshot *runtime.CompiledSnapshot) {
	if g.cluster == nil || !g.cluster.IsLeader() || snapshot == nil {
		return
	}
	payload := mapping.NewMap[string, any]()
	payload.Set("type", "route_sync")
	snapshotStatus := mapping.NewMap[string, any]()
	snapshotStatus.Set("built_at", snapshot.BuiltAt.UTC().Format(time.RFC3339Nano))
	snapshotStatus.Set("services", snapshot.Services.Len())
	snapshotStatus.Set("routes", snapshot.Routes().Len())
	snapshotStatus.Set("proxy_engine", snapshot.ProxyEngine)
	payload.Set("snapshot", snapshotStatus)
	payload.Set("routes", snapshot.QueryRoutes(runtime.RouteFilter{}))
	data, err := json.Marshal(payload)
	if err != nil {
		g.logger.Error("raft payload marshal failed", "error", err)
		return
	}
	if err := g.cluster.Apply(data, 2*time.Second); err != nil {
		g.logger.Error("raft apply failed", "error", err)
	}
}

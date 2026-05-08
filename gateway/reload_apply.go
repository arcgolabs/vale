package gateway

import (
	"context"

	"github.com/arcgolabs/vale/runtime"
)

func (g *Gateway) applyReloadSnapshot(ctx context.Context, snapshot *runtime.CompiledSnapshot) {
	if snapshot == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	current := g.runtime.Snapshot()
	diff := runtime.DiffSnapshots(current, snapshot)
	if changed := staticRuntimeChanges(current, snapshot); !changed.IsEmpty() {
		g.logger.Info("snapshot contains static runtime changes; restarting servers",
			"fields", changed,
		)
		if g.events != nil {
			if err := g.events.Publish(ctx, StaticRuntimeConfigChangedEvent{
				Fields: changed,
			}); err != nil {
				g.logger.Error("publish static runtime change event failed", "error", err)
			}
		}
		if err := g.restartServersLocked(ctx, snapshot); err != nil {
			g.logger.Error("static runtime reload failed", "fields", changed, "error", err)
			g.reload = g.reloadStatusFromSnapshot("failed", current, diff, changed.Values(), err.Error())
			g.config.OnWatchError(err)
			return
		}
		g.recordReloadSuccessLocked("restarted", snapshot, diff, changed.Values())
		return
	}
	g.runtime.Swap(snapshot)
	g.publishClusterUpdate(snapshot)
	g.runtime.ObserveReload("swapped")
	g.recordReloadSuccessLocked("swapped", snapshot, diff, nil)
	g.logger.Info("runtime snapshot swapped",
		"built_at", snapshot.BuiltAt,
		"entrypoints", snapshot.Entrypoints.Len(),
		"services", snapshot.Services.Len(),
		"routes", snapshot.Routes().Len(),
	)
}

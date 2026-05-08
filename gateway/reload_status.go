package gateway

import (
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/runtime"
)

type ReloadStatusView struct {
	State        string         `json:"state"`
	LastReload   time.Time      `json:"last_reload"`
	LastError    string         `json:"last_error,omitempty"`
	Fingerprint  string         `json:"fingerprint,omitempty"`
	BuiltAt      time.Time      `json:"built_at"`
	Routes       int            `json:"routes"`
	Services     int            `json:"services"`
	StaticFields []string       `json:"static_fields,omitempty"`
	Diff         ReloadDiffView `json:"diff"`
}

type ReloadDiffView struct {
	Routes    ReloadResourceDiffView `json:"routes"`
	Services  ReloadResourceDiffView `json:"services"`
	Endpoints ReloadResourceDiffView `json:"endpoints"`
}

type ReloadResourceDiffView struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

func (g *Gateway) recordInitialSnapshotLocked(snapshot *runtime.CompiledSnapshot) {
	g.reload = g.reloadStatusFromSnapshot("loaded", snapshot, runtime.SnapshotDiff{}, nil, "")
}

func (g *Gateway) recordReloadSuccessLocked(state string, snapshot *runtime.CompiledSnapshot, diff runtime.SnapshotDiff, staticFields []string) {
	g.reload = g.reloadStatusFromSnapshot(state, snapshot, diff, staticFields, "")
}

func (g *Gateway) recordReloadError(err error) {
	if err == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reload.State = "failed"
	g.reload.LastError = err.Error()
	g.reload.LastReload = time.Now().UTC()
}

func (g *Gateway) reloadStatus() ReloadStatusView {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.reload
}

func (g *Gateway) reloadStatusFromSnapshot(
	state string,
	snapshot *runtime.CompiledSnapshot,
	diff runtime.SnapshotDiff,
	staticFields []string,
	lastError string,
) ReloadStatusView {
	status := ReloadStatusView{
		State:        state,
		LastReload:   time.Now().UTC(),
		LastError:    lastError,
		StaticFields: staticFields,
		Diff:         reloadDiffView(diff),
	}
	if snapshot == nil {
		return status
	}
	status.Fingerprint = snapshot.Fingerprint
	status.BuiltAt = snapshot.BuiltAt
	if snapshot.Services != nil {
		status.Services = snapshot.Services.Len()
	}
	status.Routes = snapshot.Routes().Len()
	return status
}

func reloadDiffView(diff runtime.SnapshotDiff) ReloadDiffView {
	return ReloadDiffView{
		Routes:    reloadResourceDiffView(diff.Routes),
		Services:  reloadResourceDiffView(diff.Services),
		Endpoints: reloadResourceDiffView(diff.Endpoints),
	}
}

func reloadResourceDiffView(diff runtime.ResourceDiff) ReloadResourceDiffView {
	return ReloadResourceDiffView{
		Added:   listValues(diff.Added),
		Removed: listValues(diff.Removed),
		Changed: listValues(diff.Changed),
	}
}

func listValues(values *collectionlist.List[string]) []string {
	if values == nil {
		return nil
	}
	return values.Values()
}

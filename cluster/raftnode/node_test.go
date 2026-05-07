package raftnode

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/hashicorp/raft"
)

func TestFSMApplySnapshotUpdateCommand(t *testing.T) {
	t.Parallel()

	store := newFSM(nil)
	result := store.Apply(&raft.Log{
		Index: 7,
		Data:  []byte(`{"type":"snapshot_update","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":2,"routes":3,"proxy_engine":"oxy"}}`),
	})
	if err, ok := result.(error); ok {
		t.Fatal(err)
	}

	state := store.State()
	if state.Version != 7 {
		t.Fatalf("version = %d, want 7", state.Version)
	}
	if state.Snapshot == nil {
		t.Fatal("snapshot state is nil")
	}
	if state.Snapshot.Services != 2 || state.Snapshot.Routes != 3 || state.Snapshot.ProxyEngine != "oxy" {
		t.Fatalf("snapshot = %#v", state.Snapshot)
	}
}

func TestFSMApplyRouteSyncCommand(t *testing.T) {
	t.Parallel()

	store := newFSM(nil)
	result := store.Apply(&raft.Log{
		Index: 9,
		Data:  []byte(`{"type":"route_sync","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":1,"routes":1,"proxy_engine":"oxy"},"routes":[{"name":"api","entrypoint":"web","path_prefix":"/api","service":"svc"}]}`),
	})
	if err, ok := result.(error); ok {
		t.Fatal(err)
	}

	state := store.State()
	if state.Version != 9 {
		t.Fatalf("version = %d, want 9", state.Version)
	}
	if len(state.Routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(state.Routes))
	}
	if state.Routes[0].Name != "api" || state.Routes[0].PathPrefix != "/api" {
		t.Fatalf("route = %#v", state.Routes[0])
	}
}

func TestFSMApplyLegacyPayloadAsRawState(t *testing.T) {
	t.Parallel()

	store := newFSM(nil)
	store.Apply(&raft.Log{
		Index: 3,
		Data:  []byte(`{"routes":1}`),
	})
	state := store.State()
	if state.Version != 3 {
		t.Fatalf("version = %d, want 3", state.Version)
	}
	if string(state.Raw) != `{"routes":1}` {
		t.Fatalf("raw = %s", state.Raw)
	}
}

func TestFSMPersistsRouteStateWithStorxBboltx(t *testing.T) {
	t.Parallel()

	stateStore, err := openStateStore(filepath.Join(t.TempDir(), "state.bbolt"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer stateStore.Close()

	store := newFSM(stateStore)
	result := store.Apply(&raft.Log{
		Index: 15,
		Data:  []byte(`{"type":"route_sync","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":1,"routes":2,"proxy_engine":"oxy"},"routes":[{"name":"api","entrypoint":"web","path_prefix":"/api","service":"svc"},{"name":"admin","entrypoint":"web","path_prefix":"/admin","service":"admin"}]}`),
	})
	if err, ok := result.(error); ok {
		t.Fatal(err)
	}

	persisted, ok, err := stateStore.LoadState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("state was not persisted")
	}
	if persisted.Version != 15 || len(persisted.Routes) != 2 {
		t.Fatalf("persisted state = %#v", persisted)
	}
	routes, err := stateStore.LoadRoutes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 {
		t.Fatalf("persisted routes len = %d, want 2", len(routes))
	}
}

func TestFSMSnapshotRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := newFSM(nil)
	store.Apply(&raft.Log{
		Index: 11,
		Data:  []byte(`{"type":"snapshot_update","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":5,"routes":8,"proxy_engine":"oxy"}}`),
	})
	snapshot, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	sink := &memorySnapshotSink{}
	if err := snapshot.Persist(sink); err != nil {
		t.Fatal(err)
	}

	restored := newFSM(nil)
	if err := restored.Restore(io.NopCloser(bytes.NewReader(sink.Bytes()))); err != nil {
		t.Fatal(err)
	}
	state := restored.State()
	if state.Version != 11 {
		t.Fatalf("restored version = %d, want 11", state.Version)
	}
	if state.Snapshot == nil || state.Snapshot.Routes != 8 {
		t.Fatalf("restored snapshot = %#v", state.Snapshot)
	}
}

type memorySnapshotSink struct {
	bytes.Buffer
	closed   bool
	canceled bool
}

func (s *memorySnapshotSink) ID() string {
	return "memory"
}

func (s *memorySnapshotSink) Close() error {
	s.closed = true
	return nil
}

func (s *memorySnapshotSink) Cancel() error {
	s.canceled = true
	return nil
}

package raftnode

import (
	"bytes"
	"io"
	"testing"

	"github.com/hashicorp/raft"
)

func TestFSMApplySnapshotUpdateCommand(t *testing.T) {
	t.Parallel()

	store := newFSM()
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

func TestFSMApplyLegacyPayloadAsRawState(t *testing.T) {
	t.Parallel()

	store := newFSM()
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

func TestFSMSnapshotRestoreRoundTrip(t *testing.T) {
	t.Parallel()

	store := newFSM()
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

	restored := newFSM()
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

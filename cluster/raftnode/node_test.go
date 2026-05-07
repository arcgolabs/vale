package raftnode_test

import (
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/arcgolabs/vela/cluster/raftnode"
)

func TestNodeAppliesSnapshotUpdateCommand(t *testing.T) {
	node := newTestNode(t)

	mustApply(t, node, []byte(`{"type":"snapshot_update","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":2,"routes":3,"proxy_engine":"oxy"}}`))

	state := node.AppliedState()
	if state.Version == 0 {
		t.Fatal("version = 0, want raft log version")
	}
	if state.Snapshot == nil {
		t.Fatal("snapshot state is nil")
	}
	if state.Snapshot.Services != 2 || state.Snapshot.Routes != 3 || state.Snapshot.ProxyEngine != "oxy" {
		t.Fatalf("snapshot = %#v", state.Snapshot)
	}
}

func TestNodeAppliesRouteSyncCommand(t *testing.T) {
	node := newTestNode(t)

	mustApply(t, node, []byte(`{"type":"route_sync","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":1,"routes":1,"proxy_engine":"oxy"},"routes":[{"name":"api","entrypoint":"web","path_prefix":"/api","service":"svc"}]}`))

	state := node.AppliedState()
	if state.Version == 0 {
		t.Fatal("version = 0, want raft log version")
	}
	if len(state.Routes) != 1 {
		t.Fatalf("routes len = %d, want 1", len(state.Routes))
	}
	if state.Routes[0].Name != "api" || state.Routes[0].PathPrefix != "/api" {
		t.Fatalf("route = %#v", state.Routes[0])
	}
}

func TestNodeAppliesLegacyPayloadAsRawState(t *testing.T) {
	node := newTestNode(t)

	mustApply(t, node, []byte(`{"routes":1}`))

	state := node.AppliedState()
	if state.Version == 0 {
		t.Fatal("version = 0, want raft log version")
	}
	if string(state.Raw) != `{"routes":1}` {
		t.Fatalf("raw = %s", state.Raw)
	}
}

func TestNodePeersReturnsBootstrapVoter(t *testing.T) {
	node := newTestNode(t)

	peers, err := node.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 {
		t.Fatalf("peers len = %d, want 1", len(peers))
	}
	if peers[0].ID != "node-1" || peers[0].Suffrage != "Voter" {
		t.Fatalf("peer = %#v", peers[0])
	}
}

func newTestNode(t *testing.T) *raftnode.Node {
	t.Helper()

	node, err := raftnode.New(raftnode.Config{
		Enabled:   true,
		NodeID:    "node-1",
		BindAddr:  freeAddr(t),
		DataDir:   t.TempDir(),
		Bootstrap: true,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := node.Shutdown(); err != nil {
			t.Fatal(err)
		}
	})
	waitForLeader(t, node)
	return node
}

func waitForLeader(t *testing.T, node *raftnode.Node) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if node.IsLeader() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("raft node did not become leader")
}

func mustApply(t *testing.T, node *raftnode.Node, data []byte) {
	t.Helper()

	if err := node.Apply(data, time.Second); err != nil {
		t.Fatal(err)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	return listener.Addr().String()
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

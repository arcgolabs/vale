package raftnode_test

import (
	"log/slog"
	"net"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/cluster/raftnode"
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
	if state.Routes.Len() != 1 {
		t.Fatalf("routes len = %d, want 1", state.Routes.Len())
	}
	route, _ := state.Routes.GetFirst()
	if route.Name != "api" || route.PathPrefix != "/api" {
		t.Fatalf("route = %#v", route)
	}
}

func TestNodeLoadsPersistedAppliedState(t *testing.T) {
	dataDir := t.TempDir()
	bindAddr := freeAddr(t)
	node := newTestNodeWithDataDirAndBind(t, dataDir, bindAddr, true)

	mustApply(t, node, []byte(`{"type":"route_sync","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":1,"routes":1,"proxy_engine":"oxy"},"routes":[{"name":"api","entrypoint":"web","path_prefix":"/api","service":"svc"}]}`))
	if err := node.Shutdown(); err != nil {
		t.Fatal(err)
	}

	restarted := newTestNodeWithDataDirAndBind(t, dataDir, bindAddr, false)
	t.Cleanup(func() {
		if err := restarted.Shutdown(); err != nil {
			t.Fatal(err)
		}
	})

	state := restarted.AppliedState()
	if state.Snapshot == nil || state.Snapshot.Routes != 1 {
		t.Fatalf("persisted snapshot = %#v", state.Snapshot)
	}
	if state.Routes == nil || state.Routes.Len() != 1 {
		t.Fatalf("persisted routes = %#v", state.Routes)
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

func TestNodeAppliesMultiGroupCommands(t *testing.T) {
	node := newTestNodeWithGroups(t, collectionlist.NewList(
		raftnode.GroupConfig{Name: raftnode.DefaultGroupName, ID: raftnode.DefaultGroupID, Bootstrap: true},
		raftnode.GroupConfig{Name: "providers", ID: 2, Bootstrap: true},
	))
	waitForGroupLeader(t, node, "providers")

	mustApplyGroup(t, node, "providers", []byte(`{"type":"snapshot_update","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":3,"routes":5,"proxy_engine":"oxy"}}`))

	defaultState := node.AppliedState()
	if defaultState.Version != 0 {
		t.Fatalf("default group version = %d, want 0", defaultState.Version)
	}
	providerState := node.AppliedGroupState("providers")
	if providerState.Version == 0 {
		t.Fatal("provider group version = 0, want raft log version")
	}
	if providerState.Snapshot == nil || providerState.Snapshot.Services != 3 {
		t.Fatalf("provider snapshot = %#v", providerState.Snapshot)
	}
}

func TestNodePeersReturnsBootstrapVoter(t *testing.T) {
	node := newTestNode(t)

	peers, err := node.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if peers.Len() != 1 {
		t.Fatalf("peers len = %d, want 1", peers.Len())
	}
	peer, _ := peers.GetFirst()
	id, _ := peer.Get("id")
	suffrage, _ := peer.Get("suffrage")
	if id != "node-1" || suffrage != "Voter" {
		t.Fatalf("peer = %#v", peer)
	}
}

func newTestNode(t *testing.T) *raftnode.Node {
	t.Helper()

	return newTestNodeWithDataDir(t, t.TempDir(), true)
}

func newTestNodeWithDataDir(t *testing.T, dataDir string, bootstrap bool) *raftnode.Node {
	t.Helper()

	return newTestNodeWithDataDirAndBind(t, dataDir, freeAddr(t), bootstrap)
}

func newTestNodeWithDataDirAndBind(t *testing.T, dataDir, bindAddr string, bootstrap bool) *raftnode.Node {
	t.Helper()

	return newTestNodeWithConfig(t, raftnode.Config{
		Enabled:   true,
		NodeID:    "node-1",
		BindAddr:  bindAddr,
		DataDir:   dataDir,
		Bootstrap: bootstrap,
	})
}

func newTestNodeWithGroups(t *testing.T, groups *collectionlist.List[raftnode.GroupConfig]) *raftnode.Node {
	t.Helper()

	return newTestNodeWithConfig(t, raftnode.Config{
		Enabled:   true,
		NodeID:    "node-1",
		BindAddr:  freeAddr(t),
		DataDir:   t.TempDir(),
		Bootstrap: true,
		Groups:    groups,
	})
}

func newTestNodeWithConfig(t *testing.T, config raftnode.Config) *raftnode.Node {
	t.Helper()

	quietDragonboatLogs()
	node, err := raftnode.New(raftnode.Config{
		Enabled:        config.Enabled,
		NodeID:         config.NodeID,
		BindAddr:       config.BindAddr,
		DataDir:        config.DataDir,
		Bootstrap:      config.Bootstrap,
		DeploymentID:   config.DeploymentID,
		RTTMillisecond: config.RTTMillisecond,
		LogDB:          config.LogDB,
		Groups:         config.Groups,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := node.Shutdown(); err != nil {
			t.Fatal(err)
		}
	})
	if config.Bootstrap {
		waitForLeader(t, node)
	}
	return node
}

func waitForLeader(t *testing.T, node *raftnode.Node) {
	t.Helper()

	waitForGroupLeader(t, node, raftnode.DefaultGroupName)
}

func waitForGroupLeader(t *testing.T, node *raftnode.Node, group string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if node.IsGroupLeader(group) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("raft node did not become leader for group %q", group)
}

func mustApply(t *testing.T, node *raftnode.Node, data []byte) {
	t.Helper()

	if err := node.Apply(data, time.Second); err != nil {
		t.Fatal(err)
	}
}

func mustApplyGroup(t *testing.T, node *raftnode.Node, group string, data []byte) {
	t.Helper()

	if err := node.ApplyGroup(group, data, time.Second); err != nil {
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

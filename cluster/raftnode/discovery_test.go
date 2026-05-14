package raftnode_test

import (
	"context"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/cluster/raftnode"
)

func TestDiscoveryAutoAddsDiscoveredVoter(t *testing.T) {
	node1Addr := freeAddr(t)
	node2Addr := freeAddr(t)
	discovery := newFakeDiscovery(collectionlist.NewList(raftnode.DiscoveryNode{
		ID:          "node-2",
		RaftAddress: node2Addr,
		Groups:      collectionlist.NewList(raftnode.DefaultGroupName),
	}))
	node1 := newTestNodeWithConfig(t, raftnode.Config{
		NodeID:                     "node-1",
		BindAddr:                   node1Addr,
		DataDir:                    t.TempDir(),
		Bootstrap:                  true,
		Discovery:                  discovery,
		DiscoveryReconcileInterval: 20 * time.Millisecond,
		DiscoveryJoinTimeout:       time.Second,
		RTTMillisecond:             10,
	})
	_ = newTestNodeWithConfig(t, raftnode.Config{
		NodeID:                     "node-2",
		BindAddr:                   node2Addr,
		DataDir:                    t.TempDir(),
		Bootstrap:                  false,
		Discovery:                  newFakeDiscovery(collectionlist.NewList[raftnode.DiscoveryNode]()),
		DiscoveryReconcileInterval: 20 * time.Millisecond,
		DiscoveryJoinTimeout:       time.Second,
		RTTMillisecond:             10,
	})
	discovery.Trigger()
	waitForPeer(t, node1, "node-2")
}

type fakeDiscovery struct {
	peers    *collectionlist.List[raftnode.DiscoveryNode]
	onChange func()
}

func newFakeDiscovery(peers *collectionlist.List[raftnode.DiscoveryNode]) *fakeDiscovery {
	if peers == nil {
		peers = collectionlist.NewList[raftnode.DiscoveryNode]()
	}
	return &fakeDiscovery{peers: peers}
}

func (d *fakeDiscovery) Start(_ context.Context, _ raftnode.DiscoveryNode, onChange func()) error {
	d.onChange = onChange
	return nil
}

func (d *fakeDiscovery) Peers() *collectionlist.List[raftnode.DiscoveryNode] {
	return d.peers
}

func (d *fakeDiscovery) Shutdown() error {
	return nil
}

func (d *fakeDiscovery) Trigger() {
	if d.onChange != nil {
		d.onChange()
	}
}

func waitForPeer(t *testing.T, node *raftnode.Node, id string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		peers, err := node.Peers()
		if err == nil && peerListContains(peers, id) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	peers, err := node.Peers()
	if err != nil {
		t.Fatalf("get peers after wait: %v", err)
	}
	t.Fatalf("raft peer %q was not added; peer ids = %#v; status = %#v", id, peerIDs(peers), node.Status())
}

func peerListContains(peers *collectionlist.List[*raftnode.Peer], id string) bool {
	if peers == nil {
		return false
	}
	found := false
	peers.Range(func(_ int, peer *raftnode.Peer) bool {
		peerID, _ := peer.Get("id")
		if peerID == id {
			found = true
			return false
		}
		return true
	})
	return found
}

func peerIDs(peers *collectionlist.List[*raftnode.Peer]) *collectionlist.List[string] {
	ids := collectionlist.NewList[string]()
	if peers == nil {
		return ids
	}
	peers.Range(func(_ int, peer *raftnode.Peer) bool {
		peerID, _ := peer.Get("id")
		ids.Add(peerID)
		return true
	})
	return ids
}

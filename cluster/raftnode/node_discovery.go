package raftnode

import (
	"context"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

func (n *Node) startDiscovery(config Config) error {
	if config.Discovery == nil {
		return nil
	}
	interval := config.DiscoveryReconcileInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	joinTimeout := config.DiscoveryJoinTimeout
	if joinTimeout <= 0 {
		joinTimeout = 5 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	n.discovery = config.Discovery
	n.discoveryCancel = cancel
	n.discoveryDone = make(chan struct{})
	n.discoveryChanged = make(chan struct{}, 1)
	n.discoveryJoinTimeout = joinTimeout
	n.discoveryReconcilePeriod = interval
	if err := config.Discovery.Start(ctx, n.localDiscoveryNode(), n.notifyDiscoveryChanged); err != nil {
		cancel()
		return oops.
			In("raftnode").
			With("node_id", n.nodeName).
			Wrapf(err, "start discovery")
	}
	go n.runDiscoveryReconciler(ctx)
	n.notifyDiscoveryChanged()
	return nil
}

func (n *Node) localDiscoveryNode() DiscoveryNode {
	groups := collectionlist.NewList[string]()
	n.groups.Range(func(name string, _ *raftGroup) bool {
		groups.Add(name)
		return true
	})
	return DiscoveryNode{
		ID:           n.nodeName,
		RaftAddress:  n.raftAddress(),
		DeploymentID: n.deploymentID,
		Groups:       groups,
	}
}

func (n *Node) raftAddress() string {
	if n == nil || n.nodeHost == nil {
		return ""
	}
	return n.nodeHost.RaftAddress()
}

func (n *Node) notifyDiscoveryChanged() {
	if n == nil || n.discoveryChanged == nil {
		return
	}
	select {
	case n.discoveryChanged <- struct{}{}:
	default:
	}
}

func (n *Node) runDiscoveryReconciler(ctx context.Context) {
	defer close(n.discoveryDone)
	ticker := time.NewTicker(n.discoveryReconcilePeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.discoveryChanged:
			n.reconcileDiscoveryPeers(ctx)
		case <-ticker.C:
			n.reconcileDiscoveryPeers(ctx)
		}
	}
}

func (n *Node) reconcileDiscoveryPeers(ctx context.Context) {
	if n == nil || n.discovery == nil || !n.IsEnabled() {
		return
	}
	n.discovery.Peers().Range(func(_ int, peer DiscoveryNode) bool {
		n.reconcileDiscoveryPeer(ctx, peer)
		return true
	})
}

func (n *Node) reconcileDiscoveryPeer(ctx context.Context, peer DiscoveryNode) {
	if !n.shouldJoinDiscoveryPeer(peer) {
		return
	}
	groups := peer.Groups
	if groups == nil || groups.IsEmpty() {
		groups = n.localGroupNames()
	}
	groups.Range(func(_ int, group string) bool {
		group = strings.TrimSpace(group)
		if group == "" || !n.IsGroupLeader(group) {
			return true
		}
		n.ensureDiscoveryVoter(ctx, group, peer)
		return true
	})
}

func (n *Node) shouldJoinDiscoveryPeer(peer DiscoveryNode) bool {
	if strings.TrimSpace(peer.ID) == "" || strings.TrimSpace(peer.RaftAddress) == "" {
		return false
	}
	if peer.ID == n.nodeName {
		return false
	}
	if n.deploymentID != 0 && peer.DeploymentID != 0 && n.deploymentID != peer.DeploymentID {
		if n.logger != nil {
			n.logger.Warn("discovered peer skipped because deployment id differs",
				"peer_id", peer.ID,
				"peer_deployment_id", peer.DeploymentID,
				"deployment_id", n.deploymentID,
			)
		}
		return false
	}
	return true
}

func (n *Node) localGroupNames() *collectionlist.List[string] {
	groups := collectionlist.NewList[string]()
	n.groups.Range(func(name string, _ *raftGroup) bool {
		groups.Add(name)
		return true
	})
	return groups
}

func (n *Node) ensureDiscoveryVoter(ctx context.Context, group string, peer DiscoveryNode) {
	lookupCtx, lookupCancel := context.WithTimeout(ctx, n.discoveryJoinTimeout)
	peers, err := n.groupPeers(lookupCtx, group)
	lookupCancel()
	if err != nil {
		if n.logger != nil {
			n.logger.Debug("discovery membership lookup failed", "group", group, "peer_id", peer.ID, "error", err)
		}
		return
	}
	if peerAlreadyJoined(peers, peer.ID) {
		return
	}
	if n.logger != nil {
		n.logger.Info("adding discovered raft voter", "group", group, "peer_id", peer.ID, "raft_address", peer.RaftAddress)
	}
	joinCtx, cancel := context.WithTimeout(ctx, n.discoveryJoinTimeout)
	defer cancel()
	if err := n.addGroupVoter(joinCtx, group, peer.ID, peer.RaftAddress); err != nil && n.logger != nil {
		n.logger.Debug("add discovered raft voter failed",
			"group", group,
			"peer_id", peer.ID,
			"raft_address", peer.RaftAddress,
			"error", err,
		)
	}
}

func peerAlreadyJoined(peers *collectionlist.List[*Peer], id string) bool {
	if peers == nil {
		return false
	}
	joined := false
	peers.Range(func(_ int, peer *Peer) bool {
		if peer == nil {
			return true
		}
		peerID, _ := peer.Get("id")
		if peerID == id {
			joined = true
			return false
		}
		return true
	})
	return joined
}

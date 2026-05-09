// Package raftnode provides an optional Dragonboat multi-group Raft adapter for Vale.
package raftnode

import (
	"context"
	"errors"
	"log/slog"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	dragonboat "github.com/lni/dragonboat/v3"
	"github.com/samber/oops"
)

type Node struct {
	nodeHost     *dragonboat.NodeHost
	ownsNodeHost bool
	groups       *mapping.Map[string, *raftGroup]
	nodeID       uint64
	nodeName     string
	logger       *slog.Logger
}

func New(config Config, logger *slog.Logger) (*Node, error) {
	configureDragonboatLogger(logger)
	if err := prepareConfig(&config); err != nil {
		return nil, err
	}
	nodeID := stableNodeID(config.NodeID)
	nodeHost := config.NodeHost
	ownsNodeHost := nodeHost == nil
	if ownsNodeHost {
		var err error
		nodeHost, err = dragonboat.NewNodeHost(nodeHostConfig(config))
		if err != nil {
			return nil, oops.
				In("raftnode").
				With("node_id", config.NodeID, "bind_addr", config.BindAddr).
				Wrapf(err, "create dragonboat nodehost")
		}
	}
	node := &Node{
		nodeHost:     nodeHost,
		ownsNodeHost: ownsNodeHost,
		groups:       mapping.NewMap[string, *raftGroup](),
		nodeID:       nodeID,
		nodeName:     config.NodeID,
		logger:       logger,
	}
	if err := node.startGroups(config); err != nil {
		closeErr := node.Shutdown()
		return nil, oops.
			In("raftnode").
			Wrapf(errors.Join(err, closeErr), "start raft groups")
	}
	return node, nil
}

func (n *Node) IsEnabled() bool {
	return n != nil && n.nodeHost != nil
}

func (n *Node) IsLeader() bool {
	return n.IsGroupLeader(DefaultGroupName)
}

func (n *Node) IsGroupLeader(group string) bool {
	raftGroup, ok := n.group(group)
	if !ok {
		return false
	}
	leader, ready, err := n.nodeHost.GetLeaderID(raftGroup.id)
	return err == nil && ready && leader == n.nodeID
}

func (n *Node) Status() *mapping.Map[string, any] {
	status := mapping.NewMap[string, any]()
	if !n.IsEnabled() {
		status.Set("enabled", false)
		return status
	}
	status.Set("enabled", true)
	status.Set("node_id", n.nodeName)
	status.Set("numeric_node_id", n.nodeID)
	groups := mapping.NewMap[string, any]()
	n.groups.Range(func(name string, group *raftGroup) bool {
		groupStatus := mapping.NewMap[string, any]()
		leader, ready, err := n.nodeHost.GetLeaderID(group.id)
		groupStatus.Set("cluster_id", group.id)
		groupStatus.Set("leader_ready", ready)
		groupStatus.Set("leader", leader)
		groupStatus.Set("is_leader", err == nil && ready && leader == n.nodeID)
		groupStatus.Set("applied", n.AppliedGroupState(name))
		if err != nil {
			groupStatus.Set("leader_error", err.Error())
		}
		groups.Set(name, groupStatus)
		return true
	})
	status.Set("groups", groups)
	return status
}

func (n *Node) Peers() (*collectionlist.List[*Peer], error) {
	return n.GroupPeers(DefaultGroupName)
}

func (n *Node) GroupPeers(group string) (*collectionlist.List[*Peer], error) {
	if !n.IsEnabled() {
		return collectionlist.NewList[*Peer](), nil
	}
	raftGroup, ok := n.group(group)
	if !ok {
		return nil, oops.
			In("raftnode").
			With("group", group).
			New("raft group is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	membership, err := n.nodeHost.SyncGetClusterMembership(ctx, raftGroup.id)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("group", raftGroup.name).
			Wrapf(err, "get raft group membership")
	}
	raftGroup.configChangeID = membership.ConfigChangeID
	return membershipPeers(raftGroup, membership), nil
}

func (n *Node) AddVoter(id, address string, timeout time.Duration) error {
	return n.AddGroupVoter(DefaultGroupName, id, address, timeout)
}

func (n *Node) AddGroupVoter(group, id, address string, timeout time.Duration) error {
	if !n.IsEnabled() {
		return ErrNotRunning
	}
	if id == "" || address == "" {
		return oops.
			In("raftnode").
			With("group", group, "id", id, "address", address).
			New("id and address are required")
	}
	raftGroup, ok := n.group(group)
	if !ok {
		return oops.
			In("raftnode").
			With("group", group).
			New("raft group is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	configChangeID, err := n.syncConfigChangeID(ctx, raftGroup)
	if err != nil {
		return err
	}
	err = n.nodeHost.SyncRequestAddNode(ctx, raftGroup.id, stableNodeID(id), address, configChangeID)
	if err != nil {
		return oops.
			In("raftnode").
			With("group", raftGroup.name, "id", id, "address", address, "timeout", timeout.String()).
			Wrapf(err, "add raft voter")
	}
	raftGroup.memberNames.Set(stableNodeID(id), id)
	return nil
}

func (n *Node) RemoveServer(id string, timeout time.Duration) error {
	return n.RemoveGroupServer(DefaultGroupName, id, timeout)
}

func (n *Node) RemoveGroupServer(group, id string, timeout time.Duration) error {
	if !n.IsEnabled() {
		return ErrNotRunning
	}
	if id == "" {
		return oops.
			In("raftnode").
			With("group", group).
			New("id is required")
	}
	raftGroup, ok := n.group(group)
	if !ok {
		return oops.
			In("raftnode").
			With("group", group).
			New("raft group is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	configChangeID, err := n.syncConfigChangeID(ctx, raftGroup)
	if err != nil {
		return err
	}
	err = n.nodeHost.SyncRequestDeleteNode(ctx, raftGroup.id, stableNodeID(id), configChangeID)
	if err != nil {
		return oops.
			In("raftnode").
			With("group", raftGroup.name, "id", id, "timeout", timeout.String()).
			Wrapf(err, "remove raft server")
	}
	return nil
}

func (n *Node) syncConfigChangeID(ctx context.Context, group *raftGroup) (uint64, error) {
	membership, err := n.nodeHost.SyncGetClusterMembership(ctx, group.id)
	if err != nil {
		return 0, oops.
			In("raftnode").
			With("group", group.name).
			Wrapf(err, "get raft group config change id")
	}
	group.configChangeID = membership.ConfigChangeID
	return membership.ConfigChangeID, nil
}

package raftnode

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	dragonboat "github.com/lni/dragonboat/v3"
	dragonconfig "github.com/lni/dragonboat/v3/config"
	sm "github.com/lni/dragonboat/v3/statemachine"
	"github.com/samber/oops"
)

type raftGroup struct {
	name           string
	id             uint64
	initialMembers map[uint64]string
	memberNames    *mapping.Map[uint64, string]
	configChangeID uint64
}

func (n *Node) startGroups(config Config) error {
	var startErr error
	config.Groups.Range(func(_ int, groupConfig GroupConfig) bool {
		group, err := n.startGroup(config, groupConfig)
		if err != nil {
			startErr = err
			return false
		}
		n.groups.Set(group.name, group)
		return true
	})
	return startErr
}

func (n *Node) startGroup(config Config, groupConfig GroupConfig) (*raftGroup, error) {
	groupConfig = normalizeGroupConfig(config, groupConfig)
	group := &raftGroup{
		name:           groupConfig.Name,
		id:             groupConfig.ID,
		initialMembers: initialMembers(config, groupConfig),
		memberNames:    initialMemberNames(config, groupConfig),
	}
	raftConfig := configForGroup(groupConfig.ID, n.nodeID)
	createSM := func(clusterID, _ uint64) sm.IStateMachine {
		return newFSM(groupConfig.Name, clusterID, n.store)
	}
	if err := n.nodeHost.StartCluster(group.initialMembers, groupConfig.Join, createSM, raftConfig); err != nil {
		return nil, oops.
			In("raftnode").
			With("group", groupConfig.Name, "cluster_id", groupConfig.ID, "node_id", config.NodeID).
			Wrapf(err, "start dragonboat raft group")
	}
	if n.logger != nil {
		n.logger.Info("raft group started",
			"group", groupConfig.Name,
			"cluster_id", groupConfig.ID,
			"node_id", config.NodeID,
			"join", groupConfig.Join,
			"members", len(group.initialMembers),
		)
	}
	return group, nil
}

func normalizeGroupConfig(config Config, groupConfig GroupConfig) GroupConfig {
	if strings.TrimSpace(groupConfig.Name) == "" {
		groupConfig.Name = DefaultGroupName
	}
	if groupConfig.ID == 0 {
		groupConfig.ID = stableGroupID(groupConfig.Name)
	}
	if groupConfig.InitialMembers == nil || groupConfig.InitialMembers.IsEmpty() {
		groupConfig.Bootstrap = groupConfig.Bootstrap || config.Bootstrap
	}
	return groupConfig
}

func configForGroup(clusterID, nodeID uint64) dragonconfig.Config {
	return dragonconfig.Config{
		NodeID:             nodeID,
		ClusterID:          clusterID,
		CheckQuorum:        true,
		ElectionRTT:        10,
		HeartbeatRTT:       1,
		SnapshotEntries:    1024,
		CompactionOverhead: 128,
	}
}

func initialMembers(config Config, groupConfig GroupConfig) map[uint64]string {
	members := map[uint64]string{}
	if groupConfig.InitialMembers != nil && !groupConfig.InitialMembers.IsEmpty() {
		groupConfig.InitialMembers.Range(func(id, address string) bool {
			members[stableNodeID(id)] = address
			return true
		})
		return members
	}
	if groupConfig.Bootstrap {
		members[stableNodeID(config.NodeID)] = config.BindAddr
	}
	return members
}

func (n *Node) group(name string) (*raftGroup, bool) {
	if n == nil || n.groups == nil {
		return nil, false
	}
	if strings.TrimSpace(name) == "" {
		name = DefaultGroupName
	}
	return n.groups.Get(name)
}

func initialMemberNames(config Config, groupConfig GroupConfig) *mapping.Map[uint64, string] {
	names := mapping.NewMap[uint64, string]()
	if groupConfig.InitialMembers != nil && !groupConfig.InitialMembers.IsEmpty() {
		groupConfig.InitialMembers.Range(func(id, _ string) bool {
			names.Set(stableNodeID(id), id)
			return true
		})
		return names
	}
	names.Set(stableNodeID(config.NodeID), config.NodeID)
	return names
}

func membershipPeers(group *raftGroup, membership *dragonboat.Membership) *collectionlist.List[*Peer] {
	if membership == nil {
		return collectionlist.NewList[*Peer]()
	}
	peers := collectionlist.NewList[*Peer]()
	for id, address := range membership.Nodes {
		peers.Add(newPeer(peerID(group, id), address, "Voter"))
	}
	for id, address := range membership.Observers {
		peers.Add(newPeer(peerID(group, id), address, "Observer"))
	}
	for id, address := range membership.Witnesses {
		peers.Add(newPeer(peerID(group, id), address, "Witness"))
	}
	return peers
}

func newPeer(id, address, suffrage string) *Peer {
	peer := mapping.NewMapWithCapacity[string, string](3)
	peer.Set("id", id)
	peer.Set("address", address)
	peer.Set("suffrage", suffrage)
	return peer
}

func peerID(group *raftGroup, id uint64) string {
	if group != nil && group.memberNames != nil {
		if name, ok := group.memberNames.Get(id); ok {
			return name
		}
	}
	return strconv.FormatUint(id, 10)
}

func stableGroupID(name string) uint64 {
	if name == DefaultGroupName {
		return DefaultGroupID
	}
	return stableNodeID(name)
}

func stableNodeID(id string) uint64 {
	trimmed := strings.TrimSpace(id)
	if parsed, err := strconv.ParseUint(trimmed, 10, 64); err == nil && parsed > 0 {
		return parsed
	}
	hasher := fnv.New64a()
	if _, err := hasher.Write([]byte(trimmed)); err != nil {
		return 1
	}
	value := hasher.Sum64()
	if value == 0 {
		return 1
	}
	return value
}

func groupNameFromClusterID(clusterID uint64) string {
	if clusterID == DefaultGroupID {
		return DefaultGroupName
	}
	return fmt.Sprintf("group-%d", clusterID)
}

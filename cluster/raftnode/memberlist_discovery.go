package raftnode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/hashicorp/memberlist"
	"github.com/samber/oops"
)

const (
	DefaultGossipBindAddr     = "127.0.0.1:17100"
	defaultGossipJoinInterval = 5 * time.Second
)

var (
	_ Discovery                = (*MemberlistDiscovery)(nil)
	_ memberlist.Delegate      = (*MemberlistDiscovery)(nil)
	_ memberlist.EventDelegate = (*MemberlistDiscovery)(nil)
)

type MemberlistDiscoveryConfig struct {
	BindAddr      string
	AdvertiseAddr string
	Seeds         *collectionlist.List[string]
	JoinInterval  time.Duration
}

type MemberlistDiscovery struct {
	config     MemberlistDiscoveryConfig
	logger     *slog.Logger
	local      DiscoveryNode
	memberlist *memberlist.Memberlist
	peers      *mapping.ConcurrentMap[string, DiscoveryNode]
	onChange   func()
	done       chan struct{}
}

type discoveryNodeMeta struct {
	ID           string   `json:"id"`
	RaftAddress  string   `json:"raft_address"`
	DeploymentID uint64   `json:"deployment_id,omitempty"`
	Groups       []string `json:"groups,omitempty"`
}

func NewMemberlistDiscovery(config MemberlistDiscoveryConfig, logger *slog.Logger) *MemberlistDiscovery {
	if strings.TrimSpace(config.BindAddr) == "" {
		config.BindAddr = DefaultGossipBindAddr
	}
	if config.Seeds == nil {
		config.Seeds = collectionlist.NewList[string]()
	}
	if config.JoinInterval <= 0 {
		config.JoinInterval = defaultGossipJoinInterval
	}
	return &MemberlistDiscovery{
		config: config,
		logger: logger,
		peers:  mapping.NewConcurrentMap[string, DiscoveryNode](),
	}
}

func (d *MemberlistDiscovery) Start(ctx context.Context, local DiscoveryNode, onChange func()) error {
	if d == nil {
		return oops.In("raftnode").New("memberlist discovery is nil")
	}
	local = local.normalized()
	if local.ID == "" || local.RaftAddress == "" {
		return oops.
			In("raftnode").
			With("node_id", local.ID, "raft_address", local.RaftAddress).
			New("memberlist discovery requires local node id and raft address")
	}
	d.local = local
	memberlistConfig, err := d.memberlistConfig(local)
	if err != nil {
		return err
	}
	ml, err := memberlist.Create(memberlistConfig)
	if err != nil {
		return oops.
			In("raftnode").
			With("node_id", local.ID, "gossip_bind", d.config.BindAddr).
			Wrapf(err, "create memberlist discovery")
	}
	d.memberlist = ml
	d.onChange = onChange
	d.done = make(chan struct{})
	d.upsertMember(ml.LocalNode())
	go d.runSeedJoiner(ctx)
	d.notifyChange()
	return nil
}

func (d *MemberlistDiscovery) Peers() *collectionlist.List[DiscoveryNode] {
	peers := collectionlist.NewList[DiscoveryNode]()
	if d == nil || d.peers == nil {
		return peers
	}
	d.refreshMembers()
	d.peers.Range(func(_ string, peer DiscoveryNode) bool {
		if peer.ID != d.local.ID {
			peers.Add(peer)
		}
		return true
	})
	return peers
}

func (d *MemberlistDiscovery) Shutdown() error {
	if d == nil || d.memberlist == nil {
		return nil
	}
	if d.done != nil {
		<-d.done
	}
	if err := d.memberlist.Leave(time.Second); err != nil && d.logger != nil {
		d.logger.Debug("memberlist leave failed", "error", err)
	}
	if err := d.memberlist.Shutdown(); err != nil {
		return oops.In("raftnode").Wrapf(err, "shutdown memberlist discovery")
	}
	d.memberlist = nil
	return nil
}

func (d *MemberlistDiscovery) memberlistConfig(local DiscoveryNode) (*memberlist.Config, error) {
	bindAddr, bindPort, err := splitDiscoveryAddress(d.config.BindAddr, "0.0.0.0")
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("gossip_bind", d.config.BindAddr).
			Wrapf(err, "parse gossip bind address")
	}
	config := memberlist.DefaultLANConfig()
	config.Name = local.ID
	config.BindAddr = bindAddr
	config.BindPort = bindPort
	config.Delegate = d
	config.Events = d
	config.LogOutput = memberlistSlogWriter{logger: d.logger}
	if strings.TrimSpace(d.config.AdvertiseAddr) != "" {
		advertiseAddr, advertisePort, err := splitDiscoveryAddress(d.config.AdvertiseAddr, "")
		if err != nil {
			return nil, oops.
				In("raftnode").
				With("gossip_advertise", d.config.AdvertiseAddr).
				Wrapf(err, "parse gossip advertise address")
		}
		config.AdvertiseAddr = advertiseAddr
		config.AdvertisePort = advertisePort
	}
	return config, nil
}

func splitDiscoveryAddress(rawAddress, defaultHost string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(rawAddress))
	if err != nil {
		return "", 0, fmt.Errorf("split host port: %w", err)
	}
	if host == "" {
		host = defaultHost
	}
	if host == "" {
		return "", 0, errors.New("host cannot be empty")
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, fmt.Errorf("parse port: %w", err)
	}
	if port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("port %d is outside 1-65535", port)
	}
	return host, port, nil
}

func (d *MemberlistDiscovery) runSeedJoiner(ctx context.Context) {
	defer close(d.done)
	d.joinSeeds()
	ticker := time.NewTicker(d.config.JoinInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.joinSeeds()
		}
	}
}

func (d *MemberlistDiscovery) joinSeeds() {
	seeds := d.seedAddresses()
	if seeds.IsEmpty() || d.memberlist == nil {
		return
	}
	joined, err := d.memberlist.Join(seeds.Values())
	if err != nil {
		if d.logger != nil {
			d.logger.Debug("memberlist seed join failed", "seeds", seeds, "error", err)
		}
		return
	}
	if d.logger != nil && joined > 0 {
		d.logger.Info("memberlist seed join completed", "joined", joined, "seeds", seeds)
	}
	d.refreshMembers()
	d.notifyChange()
}

func (d *MemberlistDiscovery) seedAddresses() *collectionlist.List[string] {
	if d.config.Seeds == nil {
		return collectionlist.NewList[string]()
	}
	return collectionlist.FilterMapList(d.config.Seeds, func(_ int, seed string) (string, bool) {
		seed = strings.TrimSpace(seed)
		return seed, seed != ""
	})
}

func (d *MemberlistDiscovery) refreshMembers() {
	if d == nil || d.memberlist == nil {
		return
	}
	collectionlist.NewList(d.memberlist.Members()...).Range(func(_ int, node *memberlist.Node) bool {
		d.upsertMember(node)
		return true
	})
}

func (d *MemberlistDiscovery) upsertMember(node *memberlist.Node) {
	if d == nil || node == nil || d.peers == nil {
		return
	}
	peer, ok := discoveryNodeFromMember(node)
	if !ok {
		return
	}
	d.peers.Set(peer.ID, peer)
	d.notifyChange()
}

func (d *MemberlistDiscovery) removeMember(node *memberlist.Node) {
	if d == nil || node == nil || d.peers == nil {
		return
	}
	d.peers.Delete(node.Name)
	d.notifyChange()
}

func discoveryNodeFromMember(node *memberlist.Node) (DiscoveryNode, bool) {
	var meta discoveryNodeMeta
	if err := json.Unmarshal(node.Meta, &meta); err != nil {
		return DiscoveryNode{}, false
	}
	if strings.TrimSpace(meta.ID) == "" {
		meta.ID = node.Name
	}
	peer := DiscoveryNode{
		ID:           strings.TrimSpace(meta.ID),
		RaftAddress:  strings.TrimSpace(meta.RaftAddress),
		DeploymentID: meta.DeploymentID,
		Groups:       collectionlist.NewList(meta.Groups...),
	}
	return peer.normalized(), peer.ID != "" && peer.RaftAddress != ""
}

func (d *MemberlistDiscovery) notifyChange() {
	if d != nil && d.onChange != nil {
		d.onChange()
	}
}

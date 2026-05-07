// Package raftnode provides an optional HashiCorp Raft cluster adapter for Vela.
package raftnode

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/gateway"
	"github.com/hashicorp/raft"
	"github.com/samber/oops"
)

type Config struct {
	Enabled   bool
	NodeID    string
	BindAddr  string
	DataDir   string
	Bootstrap bool
}

var ErrDisabled = errors.New("raft disabled")

const (
	CommandTypeSnapshotUpdate = "snapshot_update"
	CommandTypeRouteSync      = "route_sync"
)

type Command struct {
	Type     string                            `json:"type"`
	Snapshot *SnapshotUpdate                   `json:"snapshot,omitempty"`
	Routes   *collectionlist.List[RouteRecord] `json:"routes,omitempty"`
	Raw      json.RawMessage                   `json:"raw,omitempty"`
}

type SnapshotUpdate struct {
	BuiltAt     string `json:"built_at"`
	Services    int    `json:"services"`
	Routes      int    `json:"routes"`
	ProxyEngine string `json:"proxy_engine"`
}

type State struct {
	Version   uint64                            `json:"version"`
	AppliedAt time.Time                         `json:"applied_at"`
	Snapshot  *SnapshotUpdate                   `json:"snapshot,omitempty"`
	Routes    *collectionlist.List[RouteRecord] `json:"routes,omitempty"`
	Raw       json.RawMessage                   `json:"raw,omitempty"`
}

type RouteRecord struct {
	Name       string `json:"name"`
	Entrypoint string `json:"entrypoint"`
	Host       string `json:"host,omitempty"`
	PathPrefix string `json:"path_prefix,omitempty"`
	Method     string `json:"method,omitempty"`
	Service    string `json:"service"`
}

func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		NodeID:    "node-1",
		BindAddr:  "127.0.0.1:17000",
		DataDir:   "./data/raft",
		Bootstrap: true,
	}
}

func WithCluster(config Config) gateway.Option {
	return gateway.WithClusterFactory(func(logger *slog.Logger) (gateway.Cluster, error) {
		node, err := New(config, logger)
		if errors.Is(err, ErrDisabled) {
			return nil, nil
		}
		return node, err
	})
}

type Node struct {
	raft        *raft.Raft
	fsm         *fsm
	store       *stateStore
	logStore    io.Closer
	stableStore io.Closer
	logger      *slog.Logger
}

func (n *Node) IsEnabled() bool {
	return n != nil && n.raft != nil
}

func (n *Node) IsLeader() bool {
	if !n.IsEnabled() {
		return false
	}
	return n.raft.State() == raft.Leader
}

func (n *Node) Apply(data []byte, timeout time.Duration) error {
	if !n.IsEnabled() {
		return nil
	}
	if n.logger != nil {
		n.logger.Info("raft apply started", "bytes", len(data), "timeout", timeout)
	}
	future := n.raft.Apply(data, timeout)
	err := future.Error()
	if err == nil {
		if responseErr, ok := future.Response().(error); ok {
			err = responseErr
		}
	}
	if err != nil && n.logger != nil {
		n.logger.Error("raft apply failed", "error", err)
	}
	if err != nil {
		return oops.
			In("raftnode").
			With("bytes", len(data), "timeout", timeout.String()).
			Wrapf(err, "apply raft command")
	}
	return nil
}

func (n *Node) AppliedState() State {
	if n == nil || n.fsm == nil {
		return State{}
	}
	return n.fsm.State()
}

func (n *Node) Status() *mapping.Map[string, any] {
	if !n.IsEnabled() {
		status := mapping.NewMap[string, any]()
		status.Set("enabled", false)
		return status
	}
	stats := n.raft.Stats()
	status := mapping.NewMap[string, any]()
	status.Set("enabled", true)
	status.Set("state", n.raft.State().String())
	status.Set("leader", string(n.raft.Leader()))
	status.Set("stats", stats)
	status.Set("applied", n.fsm.State())
	return status
}

type Peer struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Suffrage string `json:"suffrage"`
}

func (n *Node) Peers() (*collectionlist.List[gateway.ClusterPeer], error) {
	if !n.IsEnabled() {
		return collectionlist.NewList[gateway.ClusterPeer](), nil
	}
	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, oops.
			In("raftnode").
			Wrapf(err, "get raft configuration")
	}
	peers := collectionlist.NewListWithCapacity[gateway.ClusterPeer](len(future.Configuration().Servers))
	collectionlist.NewList(future.Configuration().Servers...).Range(func(_ int, server raft.Server) bool {
		peers.Add(gateway.ClusterPeer{
			ID:       string(server.ID),
			Address:  string(server.Address),
			Suffrage: server.Suffrage.String(),
		})
		return true
	})
	return peers, nil
}

func (n *Node) AddVoter(id, address string, timeout time.Duration) error {
	if !n.IsEnabled() {
		return ErrDisabled
	}
	if id == "" || address == "" {
		return oops.
			In("raftnode").
			With("id", id, "address", address).
			New("id and address are required")
	}
	if n.logger != nil {
		n.logger.Info("raft add voter started", "id", id, "address", address, "timeout", timeout)
	}
	err := n.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(address), 0, timeout).Error()
	if err != nil && n.logger != nil {
		n.logger.Error("raft add voter failed", "id", id, "address", address, "error", err)
	}
	if err != nil {
		return oops.
			In("raftnode").
			With("id", id, "address", address, "timeout", timeout.String()).
			Wrapf(err, "add raft voter")
	}
	return nil
}

func (n *Node) RemoveServer(id string, timeout time.Duration) error {
	if !n.IsEnabled() {
		return ErrDisabled
	}
	if id == "" {
		return oops.
			In("raftnode").
			New("id is required")
	}
	if n.logger != nil {
		n.logger.Info("raft remove server started", "id", id, "timeout", timeout)
	}
	err := n.raft.RemoveServer(raft.ServerID(id), 0, timeout).Error()
	if err != nil && n.logger != nil {
		n.logger.Error("raft remove server failed", "id", id, "error", err)
	}
	if err != nil {
		return oops.
			In("raftnode").
			With("id", id, "timeout", timeout.String()).
			Wrapf(err, "remove raft server")
	}
	return nil
}

func (n *Node) Shutdown() error {
	if !n.IsEnabled() {
		return nil
	}
	if n.logger != nil {
		n.logger.Info("raft shutdown started")
	}
	err := n.raft.Shutdown().Error()
	if err != nil && n.logger != nil {
		n.logger.Error("raft shutdown failed", "error", err)
	}
	err = errors.Join(err,
		closeRaftResource("state store", n.store),
		closeRaftResource("log store", n.logStore),
		closeRaftResource("stable store", n.stableStore),
	)
	if err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "shutdown raft node")
	}
	return nil
}

func closerFrom(value any) io.Closer {
	if closer, ok := value.(io.Closer); ok {
		return closer
	}
	return nil
}

func closeRaftResource(name string, closer io.Closer) error {
	if closer == nil {
		return nil
	}
	if err := closer.Close(); err != nil {
		return oops.
			In("raftnode").
			With("resource", name).
			Wrapf(err, "close raft resource")
	}
	return nil
}

type raftLogWriter struct {
	logger *slog.Logger
}

func newRaftLogWriter(logger *slog.Logger) io.Writer {
	if logger == nil {
		return io.Discard
	}
	return &raftLogWriter{logger: logger.With("component", "raft")}
}

func (w *raftLogWriter) Write(data []byte) (int, error) {
	line := strings.TrimSpace(string(data))
	if line != "" {
		w.logger.Info("raft log", "line", line)
	}
	return len(data), nil
}

// Package raftnode provides an optional HashiCorp Raft cluster adapter for Vela.
package raftnode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/arcgolabs/vela/gateway"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
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
	Type     string          `json:"type"`
	Snapshot *SnapshotUpdate `json:"snapshot,omitempty"`
	Routes   []RouteRecord   `json:"routes,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

type SnapshotUpdate struct {
	BuiltAt     string `json:"built_at"`
	Services    int    `json:"services"`
	Routes      int    `json:"routes"`
	ProxyEngine string `json:"proxy_engine"`
}

type State struct {
	Version   uint64          `json:"version"`
	AppliedAt time.Time       `json:"applied_at"`
	Snapshot  *SnapshotUpdate `json:"snapshot,omitempty"`
	Routes    []RouteRecord   `json:"routes,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
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
	raft   *raft.Raft
	fsm    *fsm
	store  *stateStore
	logger *slog.Logger
}

func New(config Config, logger *slog.Logger) (*Node, error) {
	if !config.Enabled {
		return nil, ErrDisabled
	}
	if config.NodeID == "" || config.BindAddr == "" || config.DataDir == "" {
		return nil, oops.
			In("raftnode").
			With("node_id", config.NodeID, "bind_addr", config.BindAddr, "data_dir", config.DataDir).
			New("raft config requires node_id, bind_addr and data_dir")
	}
	if err := os.MkdirAll(config.DataDir, 0o750); err != nil {
		return nil, oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "create raft data directory")
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.NodeID)
	logWriter := newRaftLogWriter(logger)
	raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
		Name:   "vela-raft",
		Level:  hclog.Info,
		Output: logWriter,
	})

	stateStore, err := openStateStore(filepath.Join(config.DataDir, "vela-state.bbolt"), logger)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "open raft applied state store")
	}
	storeOwned := true
	defer func() {
		if storeOwned {
			if closeErr := stateStore.Close(); closeErr != nil && logger != nil {
				logger.Error("close raft state store after setup failure", "error", closeErr)
			}
		}
	}()
	fsmStore := newFSM(stateStore)
	logStore, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(config.DataDir, "raft-log.bolt"),
	})
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("path", filepath.Join(config.DataDir, "raft-log.bolt")).
			Wrapf(err, "open raft log store")
	}
	stableStore, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(config.DataDir, "raft-stable.bolt"),
	})
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("path", filepath.Join(config.DataDir, "raft-stable.bolt")).
			Wrapf(err, "open raft stable store")
	}
	snapshotStore, err := raft.NewFileSnapshotStore(config.DataDir, 2, logWriter)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "open raft snapshot store")
	}
	transport, err := raft.NewTCPTransport(config.BindAddr, nil, 3, 10*time.Second, logWriter)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("bind_addr", config.BindAddr).
			Wrapf(err, "open raft tcp transport")
	}

	r, err := raft.NewRaft(raftConfig, fsmStore, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("node_id", config.NodeID, "bind_addr", config.BindAddr).
			Wrapf(err, "create raft node")
	}

	hasState, err := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("node_id", config.NodeID, "data_dir", config.DataDir).
			Wrapf(err, "check existing raft state")
	}
	if config.Bootstrap && !hasState {
		cfg := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(config.NodeID),
					Address: raft.ServerAddress(config.BindAddr),
				},
			},
		}
		if err := r.BootstrapCluster(cfg).Error(); err != nil && !errors.Is(err, raft.ErrCantBootstrap) {
			return nil, oops.
				In("raftnode").
				With("node_id", config.NodeID, "bind_addr", config.BindAddr).
				Wrapf(err, "bootstrap raft cluster")
		}
		if logger != nil {
			logger.Info("raft cluster bootstrapped", "node_id", config.NodeID, "bind_addr", config.BindAddr)
		}
	}

	storeOwned = false
	return &Node{
		raft:   r,
		fsm:    fsmStore,
		store:  stateStore,
		logger: logger,
	}, nil
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

func (n *Node) Status() map[string]any {
	if !n.IsEnabled() {
		return map[string]any{
			"enabled": false,
		}
	}
	stats := n.raft.Stats()
	return map[string]any{
		"enabled": true,
		"state":   n.raft.State().String(),
		"leader":  string(n.raft.Leader()),
		"stats":   stats,
		"applied": n.fsm.State(),
	}
}

type Peer struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Suffrage string `json:"suffrage"`
}

func (n *Node) Peers() ([]gateway.ClusterPeer, error) {
	if !n.IsEnabled() {
		return nil, nil
	}
	future := n.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, oops.
			In("raftnode").
			Wrapf(err, "get raft configuration")
	}
	peers := make([]gateway.ClusterPeer, 0, len(future.Configuration().Servers))
	for _, server := range future.Configuration().Servers {
		peers = append(peers, gateway.ClusterPeer{
			ID:       string(server.ID),
			Address:  string(server.Address),
			Suffrage: server.Suffrage.String(),
		})
	}
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
	if n.store != nil {
		if closeErr := n.store.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}
	if err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "shutdown raft node")
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

type fsm struct {
	mu    sync.RWMutex
	state State
	store *stateStore
}

func newFSM(store *stateStore) *fsm {
	state := State{}
	if store != nil {
		if loaded, ok, err := store.LoadState(context.Background()); err == nil && ok {
			state = loaded
		}
	}
	return &fsm{
		state: cloneState(state),
		store: store,
	}
}

func (f *fsm) Apply(log *raft.Log) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, err := applyCommand(f.state, log.Index, log.Data)
	if err != nil {
		return oops.
			In("raftnode").
			With("index", log.Index, "bytes", len(log.Data)).
			Wrapf(err, "apply raft fsm command")
	}
	if f.store != nil {
		if err := f.store.SaveState(context.Background(), state); err != nil {
			return oops.
				In("raftnode").
				With("index", log.Index, "version", state.Version).
				Wrapf(err, "persist raft fsm state")
		}
	}
	f.state = state
	return state
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, err := json.Marshal(f.state)
	if err != nil {
		return nil, oops.
			In("raftnode").
			Wrapf(err, "marshal raft fsm snapshot")
	}
	return &fsmSnapshot{data: data}, nil
}

func (f *fsm) Restore(reader io.ReadCloser) (restoreErr error) {
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && restoreErr == nil {
			restoreErr = oops.
				In("raftnode").
				Wrapf(closeErr, "close raft fsm snapshot reader")
		}
	}()
	data, err := io.ReadAll(reader)
	if err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "read raft fsm snapshot")
	}
	var state State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return oops.
				In("raftnode").
				Wrapf(err, "unmarshal raft fsm snapshot")
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = cloneState(state)
	if f.store != nil {
		if err := f.store.SaveState(context.Background(), f.state); err != nil {
			return oops.
				In("raftnode").
				With("version", f.state.Version).
				Wrapf(err, "persist restored raft fsm state")
		}
	}
	return nil
}

func (f *fsm) State() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return cloneState(f.state)
}

type fsmSnapshot struct {
	data []byte
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := io.Copy(sink, bytes.NewReader(s.data)); err != nil {
		if cancelErr := sink.Cancel(); cancelErr != nil {
			return oops.
				In("raftnode").
				Wrapf(errors.Join(err, cancelErr), "persist raft fsm snapshot and cancel sink")
		}
		return oops.
			In("raftnode").
			Wrapf(err, "persist raft fsm snapshot")
	}
	if err := sink.Close(); err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "close raft fsm snapshot sink")
	}
	return nil
}

func (s *fsmSnapshot) Release() {}

func applyCommand(current State, index uint64, data []byte) (State, error) {
	if !json.Valid(data) {
		return State{
			Version:   index,
			AppliedAt: time.Now().UTC(),
			Snapshot:  cloneSnapshot(current.Snapshot),
			Routes:    cloneRoutes(current.Routes),
			Raw:       append(json.RawMessage(nil), data...),
		}, nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return current, oops.
			In("raftnode").
			With("index", index).
			Wrapf(err, "decode raft command envelope")
	}
	if envelope.Type == "" {
		return State{
			Version:   index,
			AppliedAt: time.Now().UTC(),
			Snapshot:  cloneSnapshot(current.Snapshot),
			Routes:    cloneRoutes(current.Routes),
			Raw:       append(json.RawMessage(nil), data...),
		}, nil
	}
	command := Command{}
	if err := json.Unmarshal(data, &command); err != nil {
		return current, oops.
			In("raftnode").
			With("index", index, "command_type", envelope.Type).
			Wrapf(err, "decode raft command")
	}

	next := State{
		Version:   index,
		AppliedAt: time.Now().UTC(),
		Snapshot:  cloneSnapshot(current.Snapshot),
		Routes:    cloneRoutes(current.Routes),
		Raw:       append(json.RawMessage(nil), command.Raw...),
	}
	switch command.Type {
	case CommandTypeSnapshotUpdate:
		next.Snapshot = cloneSnapshot(command.Snapshot)
		return next, nil
	case CommandTypeRouteSync:
		next.Snapshot = cloneSnapshot(command.Snapshot)
		next.Routes = cloneRoutes(command.Routes)
		return next, nil
	default:
		return current, oops.
			In("raftnode").
			With("command_type", command.Type, "index", index).
			Errorf("unsupported raft command type %q", command.Type)
	}
}

func cloneState(state State) State {
	return State{
		Version:   state.Version,
		AppliedAt: state.AppliedAt,
		Snapshot:  cloneSnapshot(state.Snapshot),
		Routes:    cloneRoutes(state.Routes),
		Raw:       append(json.RawMessage(nil), state.Raw...),
	}
}

func cloneSnapshot(snapshot *SnapshotUpdate) *SnapshotUpdate {
	if snapshot == nil {
		return nil
	}
	copied := *snapshot
	return &copied
}

func cloneRoutes(routes []RouteRecord) []RouteRecord {
	if len(routes) == 0 {
		return nil
	}
	return append([]RouteRecord(nil), routes...)
}

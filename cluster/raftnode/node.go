package raftnode

import (
	"bytes"
	"fmt"
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
)

type Config struct {
	Enabled   bool
	NodeID    string
	BindAddr  string
	DataDir   string
	Bootstrap bool
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
		return New(config, logger)
	})
}

type Node struct {
	raft   *raft.Raft
	fsm    *fsm
	logger *slog.Logger
}

func New(config Config, logger *slog.Logger) (*Node, error) {
	if !config.Enabled {
		return nil, nil
	}
	if config.NodeID == "" || config.BindAddr == "" || config.DataDir == "" {
		return nil, fmt.Errorf("raft config requires node_id, bind_addr and data_dir")
	}
	if err := os.MkdirAll(config.DataDir, 0o755); err != nil {
		return nil, err
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.NodeID)
	logWriter := newRaftLogWriter(logger)
	raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
		Name:   "vela-raft",
		Level:  hclog.Info,
		Output: logWriter,
	})

	fsmStore := newFSM()
	logStore, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(config.DataDir, "raft-log.bolt"),
	})
	if err != nil {
		return nil, err
	}
	stableStore, err := raftboltdb.New(raftboltdb.Options{
		Path: filepath.Join(config.DataDir, "raft-stable.bolt"),
	})
	if err != nil {
		return nil, err
	}
	snapshotStore, err := raft.NewFileSnapshotStore(config.DataDir, 2, logWriter)
	if err != nil {
		return nil, err
	}
	transport, err := raft.NewTCPTransport(config.BindAddr, nil, 3, 10*time.Second, logWriter)
	if err != nil {
		return nil, err
	}

	r, err := raft.NewRaft(raftConfig, fsmStore, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, err
	}

	hasState, err := raft.HasExistingState(logStore, stableStore, snapshotStore)
	if err != nil {
		return nil, err
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
		if err := r.BootstrapCluster(cfg).Error(); err != nil && err != raft.ErrCantBootstrap {
			return nil, err
		}
		if logger != nil {
			logger.Info("raft cluster bootstrapped", "node_id", config.NodeID, "bind_addr", config.BindAddr)
		}
	}

	return &Node{
		raft:   r,
		fsm:    fsmStore,
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
	err := n.raft.Apply(data, timeout).Error()
	if err != nil && n.logger != nil {
		n.logger.Error("raft apply failed", "error", err)
	}
	return err
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
		return nil, err
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

func (n *Node) AddVoter(id string, address string, timeout time.Duration) error {
	if !n.IsEnabled() {
		return fmt.Errorf("raft disabled")
	}
	if id == "" || address == "" {
		return fmt.Errorf("id and address are required")
	}
	if n.logger != nil {
		n.logger.Info("raft add voter started", "id", id, "address", address, "timeout", timeout)
	}
	err := n.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(address), 0, timeout).Error()
	if err != nil && n.logger != nil {
		n.logger.Error("raft add voter failed", "id", id, "address", address, "error", err)
	}
	return err
}

func (n *Node) RemoveServer(id string, timeout time.Duration) error {
	if !n.IsEnabled() {
		return fmt.Errorf("raft disabled")
	}
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if n.logger != nil {
		n.logger.Info("raft remove server started", "id", id, "timeout", timeout)
	}
	err := n.raft.RemoveServer(raft.ServerID(id), 0, timeout).Error()
	if err != nil && n.logger != nil {
		n.logger.Error("raft remove server failed", "id", id, "error", err)
	}
	return err
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
	return err
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
	mu   sync.RWMutex
	data []byte
}

func newFSM() *fsm {
	return &fsm{}
}

func (f *fsm) Apply(log *raft.Log) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = append(f.data[:0], log.Data...)
	return nil
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	copied := append([]byte(nil), f.data...)
	return &fsmSnapshot{data: copied}, nil
}

func (f *fsm) Restore(reader io.ReadCloser) error {
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.data = data
	return nil
}

type fsmSnapshot struct {
	data []byte
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := io.Copy(sink, bytes.NewReader(s.data)); err != nil {
		_ = sink.Cancel()
		return err
	}
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}

package raftnode

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"github.com/samber/oops"
)

type raftResources struct {
	fsm           *fsm
	stateStore    *stateStore
	logStore      raft.LogStore
	stableStore   raft.StableStore
	snapshotStore raft.SnapshotStore
	transport     raft.Transport
}

func New(config Config, logger *slog.Logger) (*Node, error) {
	if !config.Enabled {
		return nil, ErrDisabled
	}
	if err := prepareRaftConfig(config); err != nil {
		return nil, err
	}

	raftConfig, logWriter := buildRaftConfig(config, logger)
	resources, err := openRaftResources(config, logger, logWriter)
	if err != nil {
		return nil, err
	}
	resourcesOwned := true
	defer closeRaftResourcesAfterSetupFailure(&resourcesOwned, resources, logger)

	r, err := raft.NewRaft(raftConfig, resources.fsm, resources.logStore, resources.stableStore, resources.snapshotStore, resources.transport)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("node_id", config.NodeID, "bind_addr", config.BindAddr).
			Wrapf(err, "create raft node")
	}

	if err := bootstrapRaftIfNeeded(config, logger, r, resources); err != nil {
		return nil, err
	}

	resourcesOwned = false
	return &Node{
		raft:        r,
		fsm:         resources.fsm,
		store:       resources.stateStore,
		logStore:    closerFrom(resources.logStore),
		stableStore: closerFrom(resources.stableStore),
		logger:      logger,
	}, nil
}

func closeRaftResourcesAfterSetupFailure(owned *bool, resources *raftResources, logger *slog.Logger) {
	if !*owned {
		return
	}
	logCloseFailure(logger, "close raft state store after setup failure", closeRaftResource("state store", resources.stateStore))
	logCloseFailure(logger, "close raft log store after setup failure", closeRaftResource("log store", closerFrom(resources.logStore)))
	logCloseFailure(logger, "close raft stable store after setup failure", closeRaftResource("stable store", closerFrom(resources.stableStore)))
}

func logCloseFailure(logger *slog.Logger, message string, err error) {
	if err != nil && logger != nil {
		logger.Error(message, "error", err)
	}
}

func prepareRaftConfig(config Config) error {
	if config.NodeID == "" || config.BindAddr == "" || config.DataDir == "" {
		return oops.
			In("raftnode").
			With("node_id", config.NodeID, "bind_addr", config.BindAddr, "data_dir", config.DataDir).
			New("raft config requires node_id, bind_addr and data_dir")
	}
	if err := os.MkdirAll(config.DataDir, 0o750); err != nil {
		return oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "create raft data directory")
	}
	return nil
}

func buildRaftConfig(config Config, logger *slog.Logger) (*raft.Config, io.Writer) {
	logWriter := newRaftLogWriter(logger)
	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(config.NodeID)
	raftConfig.Logger = hclog.New(&hclog.LoggerOptions{
		Name:   "vale-raft",
		Level:  hclog.Info,
		Output: logWriter,
	})
	return raftConfig, logWriter
}

func openRaftResources(config Config, logger *slog.Logger, logWriter io.Writer) (*raftResources, error) {
	stateStore, err := openStateStore(filepath.Join(config.DataDir, "vale-state.bbolt"), logger)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "open raft applied state store")
	}
	stateStoreOwned := true
	defer closeStateStoreOnResourceError(&stateStoreOwned, stateStore, logger)

	resources := &raftResources{
		fsm:        newFSM(stateStore),
		stateStore: stateStore,
	}
	if resources.logStore, err = openRaftBoltStore(filepath.Join(config.DataDir, "raft-log.bolt"), "open raft log store"); err != nil {
		return nil, err
	}
	if resources.stableStore, err = openRaftBoltStore(filepath.Join(config.DataDir, "raft-stable.bolt"), "open raft stable store"); err != nil {
		return nil, err
	}
	if resources.snapshotStore, err = openRaftSnapshotStore(config, logWriter); err != nil {
		return nil, err
	}
	if resources.transport, err = openRaftTransport(config, logWriter); err != nil {
		return nil, err
	}
	stateStoreOwned = false
	return resources, nil
}

func closeStateStoreOnResourceError(owned *bool, store *stateStore, logger *slog.Logger) {
	if !*owned {
		return
	}
	if closeErr := store.Close(); closeErr != nil && logger != nil {
		logger.Error("close raft state store after resource setup failure", "error", closeErr)
	}
}

func openRaftBoltStore(path, operation string) (*raftboltdb.BoltStore, error) {
	store, err := raftboltdb.New(raftboltdb.Options{Path: path})
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("path", path).
			Wrapf(err, "%s", operation)
	}
	return store, nil
}

func openRaftSnapshotStore(config Config, logWriter io.Writer) (raft.SnapshotStore, error) {
	store, err := raft.NewFileSnapshotStore(config.DataDir, 2, logWriter)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "open raft snapshot store")
	}
	return store, nil
}

func openRaftTransport(config Config, logWriter io.Writer) (raft.Transport, error) {
	transport, err := raft.NewTCPTransport(config.BindAddr, nil, 3, 10*time.Second, logWriter)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("bind_addr", config.BindAddr).
			Wrapf(err, "open raft tcp transport")
	}
	return transport, nil
}

func bootstrapRaftIfNeeded(config Config, logger *slog.Logger, r *raft.Raft, resources *raftResources) error {
	hasState, err := raft.HasExistingState(resources.logStore, resources.stableStore, resources.snapshotStore)
	if err != nil {
		return oops.
			In("raftnode").
			With("node_id", config.NodeID, "data_dir", config.DataDir).
			Wrapf(err, "check existing raft state")
	}
	if !config.Bootstrap || hasState {
		return nil
	}
	if err := r.BootstrapCluster(singleNodeRaftConfiguration(config)).Error(); err != nil && !errors.Is(err, raft.ErrCantBootstrap) {
		return oops.
			In("raftnode").
			With("node_id", config.NodeID, "bind_addr", config.BindAddr).
			Wrapf(err, "bootstrap raft cluster")
	}
	if logger != nil {
		logger.Info("raft cluster bootstrapped", "node_id", config.NodeID, "bind_addr", config.BindAddr)
	}
	return nil
}

func singleNodeRaftConfiguration(config Config) raft.Configuration {
	return raft.Configuration{
		Servers: []raft.Server{
			{
				ID:      raft.ServerID(config.NodeID),
				Address: raft.ServerAddress(config.BindAddr),
			},
		},
	}
}

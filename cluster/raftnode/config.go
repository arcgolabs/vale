package raftnode

import (
	"errors"
	"os"
	"path/filepath"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	dragonboat "github.com/lni/dragonboat/v3"
	dragonconfig "github.com/lni/dragonboat/v3/config"
	"github.com/samber/oops"
)

const (
	MetadataGroupName = "metadata"
	MetadataGroupID   = uint64(1)
	DataGroupName     = "data"
	DataGroupID       = uint64(2)
	DefaultGroupName  = DataGroupName
	DefaultGroupID    = DataGroupID
)

type Config struct {
	NodeID         string
	BindAddr       string
	DataDir        string
	Bootstrap      bool
	DeploymentID   uint64
	RTTMillisecond uint64
	NodeHost       *dragonboat.NodeHost
	LogDB          dragonconfig.LogDBConfig
	Groups         *collectionlist.List[GroupConfig]
}

type GroupConfig struct {
	Name           string
	ID             uint64
	Bootstrap      bool
	Join           bool
	InitialMembers *mapping.Map[string, string]
}

// ErrNotRunning is returned when a membership operation targets a stopped node.
var ErrNotRunning = errors.New("raft node is not running")

func DefaultConfig() Config {
	return Config{
		NodeID:    "node-1",
		BindAddr:  "127.0.0.1:17000",
		DataDir:   "./data/raft",
		Bootstrap: true,
		LogDB:     dragonconfig.GetTinyMemLogDBConfig(),
		Groups:    defaultGroups(true),
	}
}

func prepareConfig(config *Config) error {
	if err := requireNodeID(config.NodeID); err != nil {
		return err
	}
	if err := prepareNodeHostBoundary(config); err != nil {
		return err
	}
	if config.Groups == nil || config.Groups.IsEmpty() {
		config.Groups = defaultGroups(config.Bootstrap)
	}
	return requireBootstrapAddress(*config)
}

func requireNodeID(nodeID string) error {
	if nodeID == "" {
		return oops.
			In("raftnode").
			With("node_id", nodeID).
			New("raft config requires node_id")
	}
	return nil
}

func prepareNodeHostBoundary(config *Config) error {
	if config.NodeHost != nil {
		if config.BindAddr == "" {
			config.BindAddr = config.NodeHost.RaftAddress()
		}
		return nil
	}
	if config.BindAddr == "" || config.DataDir == "" {
		return oops.
			In("raftnode").
			With("node_id", config.NodeID, "bind_addr", config.BindAddr, "data_dir", config.DataDir).
			New("raft config requires bind_addr and data_dir when nodehost is not supplied")
	}
	if err := os.MkdirAll(config.DataDir, 0o750); err != nil {
		return oops.
			In("raftnode").
			With("data_dir", config.DataDir).
			Wrapf(err, "create raft data directory")
	}
	return nil
}

func requireBootstrapAddress(config Config) error {
	if config.BindAddr != "" || !hasBootstrapGroup(config) {
		return nil
	}
	return oops.
		In("raftnode").
		With("node_id", config.NodeID).
		New("raft config requires bind_addr or nodehost raft address for bootstrap groups")
}

func hasBootstrapGroup(config Config) bool {
	bootstrap := false
	config.Groups.Range(func(_ int, group GroupConfig) bool {
		group = normalizeGroupConfig(config, group)
		if group.Bootstrap {
			bootstrap = true
			return false
		}
		return true
	})
	return bootstrap
}

func defaultGroups(bootstrap bool) *collectionlist.List[GroupConfig] {
	return collectionlist.NewList(
		GroupConfig{Name: MetadataGroupName, ID: MetadataGroupID, Bootstrap: bootstrap},
		GroupConfig{Name: DataGroupName, ID: DataGroupID, Bootstrap: bootstrap},
	)
}

// NodeHostConfig returns the Dragonboat NodeHost configuration used by Vale
// when Config.NodeHost is not supplied.
func NodeHostConfig(cfg Config) dragonconfig.NodeHostConfig {
	logDB := cfg.LogDB
	if logDB.IsEmpty() {
		logDB = dragonconfig.GetTinyMemLogDBConfig()
	}
	rtt := cfg.RTTMillisecond
	if rtt == 0 {
		rtt = 100
	}
	return dragonconfig.NodeHostConfig{
		DeploymentID:   cfg.DeploymentID,
		NodeHostDir:    filepath.Join(cfg.DataDir, "nodehost"),
		WALDir:         filepath.Join(cfg.DataDir, "wal"),
		RTTMillisecond: rtt,
		RaftAddress:    cfg.BindAddr,
		Expert: dragonconfig.ExpertConfig{
			LogDB: logDB,
		},
	}
}

func nodeHostConfig(cfg Config) dragonconfig.NodeHostConfig {
	return NodeHostConfig(cfg)
}

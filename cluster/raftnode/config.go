package raftnode

import (
	"errors"
	"os"
	"path/filepath"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	dragonconfig "github.com/lni/dragonboat/v3/config"
	"github.com/samber/oops"
)

const (
	DefaultGroupName = "routes"
	DefaultGroupID   = uint64(1)
)

type Config struct {
	Enabled        bool
	NodeID         string
	BindAddr       string
	DataDir        string
	Bootstrap      bool
	DeploymentID   uint64
	RTTMillisecond uint64
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

var ErrDisabled = errors.New("raft disabled")

func DefaultConfig() Config {
	return Config{
		Enabled:   false,
		NodeID:    "node-1",
		BindAddr:  "127.0.0.1:17000",
		DataDir:   "./data/raft",
		Bootstrap: true,
		LogDB:     dragonconfig.GetTinyMemLogDBConfig(),
		Groups: collectionlist.NewList(GroupConfig{
			Name:      DefaultGroupName,
			ID:        DefaultGroupID,
			Bootstrap: true,
		}),
	}
}

func prepareConfig(config *Config) error {
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
	if config.Groups == nil || config.Groups.IsEmpty() {
		config.Groups = collectionlist.NewList(GroupConfig{
			Name:      DefaultGroupName,
			ID:        DefaultGroupID,
			Bootstrap: config.Bootstrap,
		})
	}
	return nil
}

func nodeHostConfig(cfg Config) dragonconfig.NodeHostConfig {
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

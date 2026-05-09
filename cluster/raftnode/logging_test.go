package raftnode_test

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	dragonlogger "github.com/lni/dragonboat/v3/logger"
)

func quietDragonboatLogs() {
	collectionlist.NewList("config", "dragonboat", "raft", "rsm", "logdb", "transport", "grpc", "pebblekv").Range(func(_ int, name string) bool {
		dragonlogger.GetLogger(name).SetLevel(dragonlogger.CRITICAL)
		return true
	})
}

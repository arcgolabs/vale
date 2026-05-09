package raftnode

import (
	"encoding/json"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

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

type Peer = mapping.Map[string, string]

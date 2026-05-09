package raftnode

import (
	"encoding/json"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

const (
	CommandTypeSnapshotUpdate         = "snapshot_update"
	CommandTypeRouteSync              = "route_sync"
	CommandTypeCertificateStore       = "certificate_store"
	CommandTypeCertificateDelete      = "certificate_delete"
	CommandTypeCertificateLockAcquire = "certificate_lock_acquire"
	CommandTypeCertificateLockRelease = "certificate_lock_release"
)

type Command struct {
	Type        string                            `json:"type"`
	Snapshot    *SnapshotUpdate                   `json:"snapshot,omitempty"`
	Routes      *collectionlist.List[RouteRecord] `json:"routes,omitempty"`
	Certificate *CertificateRecord                `json:"certificate,omitempty"`
	Lock        *CertificateLockCommand           `json:"lock,omitempty"`
	Raw         json.RawMessage                   `json:"raw,omitempty"`
}

type SnapshotUpdate struct {
	BuiltAt     string `json:"built_at"`
	Services    int    `json:"services"`
	Routes      int    `json:"routes"`
	ProxyEngine string `json:"proxy_engine"`
}

type State struct {
	Version      uint64                                      `json:"version"`
	AppliedAt    time.Time                                   `json:"applied_at"`
	Snapshot     *SnapshotUpdate                             `json:"snapshot,omitempty"`
	Routes       *collectionlist.List[RouteRecord]           `json:"routes,omitempty"`
	Certificates *collectionlist.List[CertificateRecord]     `json:"certificates,omitempty"`
	Locks        *collectionlist.List[CertificateLockRecord] `json:"locks,omitempty"`
	Raw          json.RawMessage                             `json:"raw,omitempty"`
}

type RouteRecord struct {
	Name       string `json:"name"`
	Entrypoint string `json:"entrypoint"`
	Host       string `json:"host,omitempty"`
	PathPrefix string `json:"path_prefix,omitempty"`
	Method     string `json:"method,omitempty"`
	Service    string `json:"service"`
}

type CertificateRecord struct {
	Key      string    `json:"key"`
	Value    []byte    `json:"value,omitempty"`
	Modified time.Time `json:"modified,omitzero"`
}

type CertificateLockCommand struct {
	Name        string    `json:"name"`
	Owner       string    `json:"owner"`
	RequestedAt time.Time `json:"requested_at,omitzero"`
	ExpiresAt   time.Time `json:"expires_at,omitzero"`
}

type CertificateLockRecord struct {
	Name      string    `json:"name"`
	Owner     string    `json:"owner"`
	ExpiresAt time.Time `json:"expires_at,omitzero"`
}

type CommandResult struct {
	OK      bool   `json:"ok"`
	Reason  string `json:"reason,omitempty"`
	Version uint64 `json:"version,omitempty"`
}

type Peer = mapping.Map[string, string]

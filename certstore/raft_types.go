package certstore

import (
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

const (
	RaftCommandCertificateStore  = "certificate_store"
	RaftCommandCertificateDelete = "certificate_delete"
	RaftCommandLockAcquire       = "certificate_lock_acquire"
	RaftCommandLockRelease       = "certificate_lock_release"
	DefaultRaftGroup             = "certificates"
)

type RaftClient interface {
	ProposeGroup(group string, data []byte, timeout time.Duration) ([]byte, error)
	AppliedGroupStateJSON(group string, timeout time.Duration) ([]byte, error)
}

type RaftStorageConfig struct {
	Client     RaftClient
	Group      string
	Timeout    time.Duration
	LockTTL    time.Duration
	Owner      string
	Projection *Projection
}

type RaftStorage struct {
	client     RaftClient
	group      string
	timeout    time.Duration
	lockTTL    time.Duration
	owner      string
	mu         sync.RWMutex
	projection *Projection
}

type raftCommand struct {
	Type        string             `json:"type"`
	Certificate *raftCertificateKV `json:"certificate,omitempty"`
	Lock        *raftLockCommand   `json:"lock,omitempty"`
}

type raftCertificateKV struct {
	Key      string    `json:"key"`
	Value    []byte    `json:"value,omitempty"`
	Modified time.Time `json:"modified,omitzero"`
}

type raftLockCommand struct {
	Name        string    `json:"name"`
	Owner       string    `json:"owner"`
	RequestedAt time.Time `json:"requested_at,omitzero"`
	ExpiresAt   time.Time `json:"expires_at,omitzero"`
}

type raftCommandResult struct {
	OK      bool   `json:"ok"`
	Reason  string `json:"reason,omitempty"`
	Version uint64 `json:"version,omitempty"`
}

type raftStateView struct {
	Certificates *collectionlist.List[raftCertificateKV] `json:"certificates,omitempty"`
}

package certstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"testing"
	"time"
	"uuid"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/certstore"
)

func TestNewRaftStorageGeneratesUniqueUUIDOwner(t *testing.T) {
	t.Parallel()

	first := raftStorageOwner(t, certstore.NewRaftStorage(certstore.RaftStorageConfig{}))
	second := raftStorageOwner(t, certstore.NewRaftStorage(certstore.RaftStorageConfig{}))

	if _, err := uuid.Parse(first); err != nil {
		t.Fatalf("first default owner %q is not a UUID: %v", first, err)
	}
	if _, err := uuid.Parse(second); err != nil {
		t.Fatalf("second default owner %q is not a UUID: %v", second, err)
	}
	if first == second {
		t.Fatalf("default owners are equal: %q", first)
	}
}

func TestNewRaftStoragePreservesExplicitOwner(t *testing.T) {
	t.Parallel()

	const owner = "node-a"
	storage := certstore.NewRaftStorage(certstore.RaftStorageConfig{Owner: owner})

	if got := raftStorageOwner(t, storage); got != owner {
		t.Fatalf("owner = %q, want %q", got, owner)
	}
}

func raftStorageOwner(t *testing.T, storage *certstore.RaftStorage) string {
	t.Helper()

	owner, ok := storage.Status().Get("owner")
	if !ok {
		t.Fatal("raft storage status has no owner")
	}
	value, ok := owner.(string)
	if !ok {
		t.Fatalf("raft storage status owner has type %T, want string", owner)
	}
	return value
}

func TestRaftStorageProposesAndRefreshesProjection(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newFakeRaftClient()
	storage := certstore.NewRaftStorage(certstore.RaftStorageConfig{
		Client:  client,
		Timeout: time.Second,
		Owner:   "node-a",
	})

	if err := storage.Store(ctx, "certificates/example/cert.pem", []byte("cert")); err != nil {
		t.Fatalf("store raft certificate: %v", err)
	}
	loaded, err := storage.Load(ctx, "certificates/example/cert.pem")
	if err != nil {
		t.Fatalf("load raft certificate: %v", err)
	}
	if string(loaded) != "cert" {
		t.Fatalf("loaded certificate = %q, want cert", loaded)
	}
	if client.proposed.Len() == 0 {
		t.Fatalf("expected raft proposal")
	}
}

func TestRaftStorageLockUsesRaftResult(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newFakeRaftClient()
	storage := certstore.NewRaftStorage(certstore.RaftStorageConfig{
		Client:  client,
		Timeout: time.Second,
		Owner:   "node-a",
	})

	if err := storage.Lock(ctx, "acme/example"); err != nil {
		t.Fatalf("lock raft certificate storage: %v", err)
	}
	if err := storage.Unlock(ctx, "acme/example"); err != nil {
		t.Fatalf("unlock raft certificate storage: %v", err)
	}
	client.rejectLocks = true
	lockCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	err := storage.Lock(lockCtx, "acme/example")
	if err == nil {
		t.Fatalf("lock with rejected raft result succeeded")
	}
}

func TestRaftStorageLoadMissingPreservesNotExist(t *testing.T) {
	t.Parallel()

	storage := certstore.NewRaftStorage(certstore.RaftStorageConfig{
		Client: newFakeRaftClient(),
		Owner:  "node-a",
	})
	_, err := storage.Load(context.Background(), "missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing load error = %v, want fs.ErrNotExist", err)
	}
}

type fakeRaftClient struct {
	certificates *collectionlist.List[fakeCertificate]
	proposed     *collectionlist.List[string]
	rejectLocks  bool
}

type fakeCertificate struct {
	Key      string    `json:"key"`
	Value    []byte    `json:"value,omitempty"`
	Modified time.Time `json:"modified,omitzero"`
}

type fakeRaftState struct {
	Certificates *collectionlist.List[fakeCertificate] `json:"certificates,omitempty"`
}

func newFakeRaftClient() *fakeRaftClient {
	return &fakeRaftClient{
		certificates: collectionlist.NewList[fakeCertificate](),
		proposed:     collectionlist.NewList[string](),
	}
}

func (c *fakeRaftClient) ProposeGroup(_ string, data []byte, _ time.Duration) ([]byte, error) {
	c.proposed.Add(string(data))
	var command struct {
		Type        string           `json:"type"`
		Certificate *fakeCertificate `json:"certificate,omitempty"`
	}
	if err := json.Unmarshal(data, &command); err != nil {
		return nil, fmt.Errorf("decode fake raft command: %w", err)
	}
	switch command.Type {
	case certstore.RaftCommandCertificateStore:
		c.certificates.RemoveIf(func(record fakeCertificate) bool {
			return record.Key == command.Certificate.Key
		})
		c.certificates.Add(*command.Certificate)
	case certstore.RaftCommandCertificateDelete:
		c.certificates.RemoveIf(func(record fakeCertificate) bool {
			return record.Key == command.Certificate.Key
		})
	case certstore.RaftCommandLockAcquire, certstore.RaftCommandLockRelease:
		if c.rejectLocks {
			return []byte(`{"ok":false,"reason":"held"}`), nil
		}
		return []byte(`{"ok":true}`), nil
	}
	return []byte(`{"ok":true}`), nil
}

func (c *fakeRaftClient) AppliedGroupStateJSON(_ string, _ time.Duration) ([]byte, error) {
	data, err := json.Marshal(fakeRaftState{Certificates: c.certificates})
	if err != nil {
		return nil, fmt.Errorf("marshal fake raft state: %w", err)
	}
	return data, nil
}

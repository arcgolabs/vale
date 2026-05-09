package certstore

import (
	"context"
	"fmt"
	"io/fs"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

// Storage is Vale's certificate key-value storage boundary.
//
// Keys use slash-separated path semantics. Files are terminal keys with bytes,
// while directories are implicit prefixes. Implementations must be safe for
// concurrent use.
type Storage interface {
	Locker

	Store(ctx context.Context, key string, value []byte) error
	Load(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) bool
	List(ctx context.Context, prefix string, recursive bool) (*collectionlist.List[string], error)
	Stat(ctx context.Context, key string) (KeyInfo, error)
}

// Locker coordinates ACME work such as certificate issuance and renewal.
// A Raft-backed implementation should make this a cluster-wide lock.
type Locker interface {
	Lock(ctx context.Context, name string) error
	Unlock(ctx context.Context, name string) error
}

// KeyInfo describes a terminal file or an implicit directory key.
type KeyInfo struct {
	Key        string
	Modified   time.Time
	Size       int64
	IsTerminal bool
}

// ErrNotExist is returned when a key is absent.
var ErrNotExist = fs.ErrNotExist

func validateFileKey(key string) error {
	if cleanKey(key) == "" {
		return oops.
			In("certstore").
			New("storage key cannot be empty")
	}
	return nil
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("certstore context canceled: %w", ctx.Err())
	default:
		return nil
	}
}

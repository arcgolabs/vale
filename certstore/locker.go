package certstore

import (
	"context"
	"fmt"

	collectionmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
)

// LocalLocker is a process-local named lock table. It is useful for standalone
// mode and tests; clustered storage should replace it with a distributed lock.
type LocalLocker struct {
	locks *collectionmapping.ConcurrentMap[string, *localLock]
}

type localLock struct {
	token chan struct{}
}

func NewLocalLocker() *LocalLocker {
	return &LocalLocker{
		locks: collectionmapping.NewConcurrentMap[string, *localLock](),
	}
}

func (l *LocalLocker) Lock(ctx context.Context, name string) error {
	name = cleanKey(name)
	if name == "" {
		return oops.
			In("certstore").
			New("lock name cannot be empty")
	}
	if err := contextErr(ctx); err != nil {
		return err
	}
	lock := l.lockFor(name)
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	select {
	case lock.token <- struct{}{}:
		return nil
	case <-done:
		return fmt.Errorf("lock %q context canceled: %w", name, ctx.Err())
	}
}

func (l *LocalLocker) Unlock(_ context.Context, name string) error {
	name = cleanKey(name)
	if name == "" {
		return oops.
			In("certstore").
			New("lock name cannot be empty")
	}
	lock := l.lockFor(name)
	select {
	case <-lock.token:
		return nil
	default:
		return oops.
			In("certstore").
			With("name", name).
			New("lock is not held")
	}
}

func (l *LocalLocker) lockFor(name string) *localLock {
	if l.locks == nil {
		l.locks = collectionmapping.NewConcurrentMap[string, *localLock]()
	}
	lock, _ := l.locks.GetOrCompute(name, func() *localLock {
		return &localLock{token: make(chan struct{}, 1)}
	})
	return lock
}

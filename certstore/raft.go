package certstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
)

func NewRaftStorage(config RaftStorageConfig) *RaftStorage {
	group := config.Group
	if group == "" {
		group = DefaultRaftGroup
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	lockTTL := config.LockTTL
	if lockTTL <= 0 {
		lockTTL = 10 * time.Minute
	}
	projection := config.Projection
	if projection == nil {
		projection = NewProjection()
	}
	owner := config.Owner
	if owner == "" {
		owner = strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return &RaftStorage{
		client:     config.Client,
		group:      group,
		timeout:    timeout,
		lockTTL:    lockTTL,
		owner:      owner,
		projection: projection,
	}
}

func (s *RaftStorage) Store(ctx context.Context, key string, value []byte) error {
	key = cleanKey(key)
	if err := validateFileKey(key); err != nil {
		return err
	}
	command := raftCommand{
		Type: RaftCommandCertificateStore,
		Certificate: &raftCertificateKV{
			Key:      key,
			Value:    value,
			Modified: time.Now().UTC(),
		},
	}
	if _, err := s.propose(ctx, command); err != nil {
		return err
	}
	return s.refresh(ctx)
}

func (s *RaftStorage) Load(ctx context.Context, key string) ([]byte, error) {
	if err := s.refresh(ctx); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return s.currentProjection().Load(ctx, key)
}

func (s *RaftStorage) Delete(ctx context.Context, key string) error {
	key = cleanKey(key)
	command := raftCommand{
		Type:        RaftCommandCertificateDelete,
		Certificate: &raftCertificateKV{Key: key},
	}
	if _, err := s.propose(ctx, command); err != nil {
		return err
	}
	return s.refresh(ctx)
}

func (s *RaftStorage) Exists(ctx context.Context, key string) bool {
	if err := s.refresh(ctx); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return s.currentProjection().Exists(ctx, key)
	}
	return s.currentProjection().Exists(ctx, key)
}

func (s *RaftStorage) List(ctx context.Context, prefix string, recursive bool) (*collectionlist.List[string], error) {
	if err := s.refresh(ctx); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return s.currentProjection().List(ctx, prefix, recursive)
}

func (s *RaftStorage) Stat(ctx context.Context, key string) (KeyInfo, error) {
	if err := s.refresh(ctx); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return KeyInfo{}, err
	}
	return s.currentProjection().Stat(ctx, key)
}

func (s *RaftStorage) Lock(ctx context.Context, name string) error {
	name = cleanKey(name)
	if name == "" {
		return oops.
			In("certstore").
			New("lock name cannot be empty")
	}
	for {
		if err := contextErr(ctx); err != nil {
			return err
		}
		now := time.Now().UTC()
		result, err := s.propose(ctx, raftCommand{
			Type: RaftCommandLockAcquire,
			Lock: &raftLockCommand{
				Name:        name,
				Owner:       s.owner,
				RequestedAt: now,
				ExpiresAt:   now.Add(s.lockTTL),
			},
		})
		if err != nil {
			return err
		}
		if result.OK {
			return nil
		}
		if err := sleepContext(ctx, 50*time.Millisecond); err != nil {
			return err
		}
	}
}

func (s *RaftStorage) Unlock(ctx context.Context, name string) error {
	name = cleanKey(name)
	if name == "" {
		return oops.
			In("certstore").
			New("lock name cannot be empty")
	}
	result, err := s.propose(ctx, raftCommand{
		Type: RaftCommandLockRelease,
		Lock: &raftLockCommand{
			Name:  name,
			Owner: s.owner,
		},
	})
	if err != nil {
		return err
	}
	if !result.OK {
		return oops.
			In("certstore").
			With("name", name, "reason", result.Reason).
			New("raft lock is not held")
	}
	return nil
}

func (s *RaftStorage) refresh(ctx context.Context) error {
	if err := contextErr(ctx); err != nil {
		return err
	}
	if s.client == nil {
		return oops.
			In("certstore").
			New("raft storage client is not configured")
	}
	data, err := s.client.AppliedGroupStateJSON(s.group, s.timeout)
	if err != nil {
		return oops.
			In("certstore").
			With("group", s.group, "timeout", s.timeout.String()).
			Wrapf(err, "read raft certificate state")
	}
	var state raftStateView
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return oops.
				In("certstore").
				With("group", s.group, "bytes", len(data)).
				Wrapf(err, "decode raft certificate state")
		}
	}
	objects := collectionlist.NewList[Object]()
	if state.Certificates != nil {
		state.Certificates.Range(func(_ int, record raftCertificateKV) bool {
			objects.Add(Object(record))
			return true
		})
	}
	s.mu.Lock()
	s.projection = NewProjection(objects.Values()...)
	s.mu.Unlock()
	return nil
}

func (s *RaftStorage) propose(ctx context.Context, command raftCommand) (raftCommandResult, error) {
	if err := contextErr(ctx); err != nil {
		return raftCommandResult{}, err
	}
	if s.client == nil {
		return raftCommandResult{}, oops.
			In("certstore").
			New("raft storage client is not configured")
	}
	data, err := json.Marshal(command)
	if err != nil {
		return raftCommandResult{}, oops.
			In("certstore").
			With("command_type", command.Type).
			Wrapf(err, "marshal raft certificate command")
	}
	resultData, err := s.client.ProposeGroup(s.group, data, s.timeout)
	if err != nil {
		return raftCommandResult{}, oops.
			In("certstore").
			With("group", s.group, "command_type", command.Type, "timeout", s.timeout.String()).
			Wrapf(err, "propose raft certificate command")
	}
	if len(resultData) == 0 {
		return raftCommandResult{OK: true}, nil
	}
	var result raftCommandResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		return raftCommandResult{}, oops.
			In("certstore").
			With("group", s.group, "command_type", command.Type, "bytes", len(resultData)).
			Wrapf(err, "decode raft certificate command result")
	}
	return result, nil
}

func (s *RaftStorage) Status() *mapping.Map[string, any] {
	status := mapping.NewMap[string, any]()
	status.Set("group", s.group)
	status.Set("owner", s.owner)
	status.Set("timeout", s.timeout.String())
	status.Set("lock_ttl", s.lockTTL.String())
	return status
}

func (s *RaftStorage) currentProjection() *Projection {
	s.mu.RLock()
	projection := s.projection
	s.mu.RUnlock()
	if projection != nil {
		return projection
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.projection == nil {
		s.projection = NewProjection()
	}
	return s.projection
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}
	select {
	case <-timer.C:
		return nil
	case <-done:
		return fmt.Errorf("sleep context canceled: %w", ctx.Err())
	}
}

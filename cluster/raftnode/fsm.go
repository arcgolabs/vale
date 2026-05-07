package raftnode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/samber/oops"
)

type fsm struct {
	mu    sync.RWMutex
	state State
	store *stateStore
}

func newFSM(store *stateStore) *fsm {
	state := State{}
	if store != nil {
		if loaded, ok, err := store.LoadState(context.Background()); err == nil && ok {
			state = loaded
		}
	}
	return &fsm{
		state: cloneState(state),
		store: store,
	}
}

func (f *fsm) Apply(log *raft.Log) any {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, err := applyCommand(f.state, log.Index, log.Data)
	if err != nil {
		return oops.
			In("raftnode").
			With("index", log.Index, "bytes", len(log.Data)).
			Wrapf(err, "apply raft fsm command")
	}
	if f.store != nil {
		if err := f.store.SaveState(context.Background(), state); err != nil {
			return oops.
				In("raftnode").
				With("index", log.Index, "version", state.Version).
				Wrapf(err, "persist raft fsm state")
		}
	}
	f.state = state
	return state
}

func (f *fsm) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	data, err := json.Marshal(f.state)
	if err != nil {
		return nil, oops.
			In("raftnode").
			Wrapf(err, "marshal raft fsm snapshot")
	}
	return &fsmSnapshot{data: data}, nil
}

func (f *fsm) Restore(reader io.ReadCloser) (restoreErr error) {
	defer func() {
		if closeErr := reader.Close(); closeErr != nil && restoreErr == nil {
			restoreErr = oops.
				In("raftnode").
				Wrapf(closeErr, "close raft fsm snapshot reader")
		}
	}()
	data, err := io.ReadAll(reader)
	if err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "read raft fsm snapshot")
	}
	var state State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return oops.
				In("raftnode").
				Wrapf(err, "unmarshal raft fsm snapshot")
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state = cloneState(state)
	if f.store != nil {
		if err := f.store.SaveState(context.Background(), f.state); err != nil {
			return oops.
				In("raftnode").
				With("version", f.state.Version).
				Wrapf(err, "persist restored raft fsm state")
		}
	}
	return nil
}

func (f *fsm) State() State {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return cloneState(f.state)
}

type fsmSnapshot struct {
	data []byte
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	if _, err := io.Copy(sink, bytes.NewReader(s.data)); err != nil {
		if cancelErr := sink.Cancel(); cancelErr != nil {
			return oops.
				In("raftnode").
				Wrapf(errors.Join(err, cancelErr), "persist raft fsm snapshot and cancel sink")
		}
		return oops.
			In("raftnode").
			Wrapf(err, "persist raft fsm snapshot")
	}
	if err := sink.Close(); err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "close raft fsm snapshot sink")
	}
	return nil
}

func (s *fsmSnapshot) Release() {}

func applyCommand(current State, index uint64, data []byte) (State, error) {
	if !json.Valid(data) {
		return State{
			Version:   index,
			AppliedAt: time.Now().UTC(),
			Snapshot:  cloneSnapshot(current.Snapshot),
			Routes:    cloneRoutes(current.Routes),
			Raw:       append(json.RawMessage(nil), data...),
		}, nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return current, oops.
			In("raftnode").
			With("index", index).
			Wrapf(err, "decode raft command envelope")
	}
	if envelope.Type == "" {
		return State{
			Version:   index,
			AppliedAt: time.Now().UTC(),
			Snapshot:  cloneSnapshot(current.Snapshot),
			Routes:    cloneRoutes(current.Routes),
			Raw:       append(json.RawMessage(nil), data...),
		}, nil
	}
	command := Command{}
	if err := json.Unmarshal(data, &command); err != nil {
		return current, oops.
			In("raftnode").
			With("index", index, "command_type", envelope.Type).
			Wrapf(err, "decode raft command")
	}

	next := State{
		Version:   index,
		AppliedAt: time.Now().UTC(),
		Snapshot:  cloneSnapshot(current.Snapshot),
		Routes:    cloneRoutes(current.Routes),
		Raw:       append(json.RawMessage(nil), command.Raw...),
	}
	switch command.Type {
	case CommandTypeSnapshotUpdate:
		next.Snapshot = cloneSnapshot(command.Snapshot)
		return next, nil
	case CommandTypeRouteSync:
		next.Snapshot = cloneSnapshot(command.Snapshot)
		next.Routes = cloneRoutes(command.Routes)
		return next, nil
	default:
		return current, oops.
			In("raftnode").
			With("command_type", command.Type, "index", index).
			Errorf("unsupported raft command type %q", command.Type)
	}
}

func cloneState(state State) State {
	return State{
		Version:   state.Version,
		AppliedAt: state.AppliedAt,
		Snapshot:  cloneSnapshot(state.Snapshot),
		Routes:    cloneRoutes(state.Routes),
		Raw:       append(json.RawMessage(nil), state.Raw...),
	}
}

func cloneSnapshot(snapshot *SnapshotUpdate) *SnapshotUpdate {
	if snapshot == nil {
		return nil
	}
	copied := *snapshot
	return &copied
}

func cloneRoutes(routes []RouteRecord) []RouteRecord {
	if len(routes) == 0 {
		return nil
	}
	return append([]RouteRecord(nil), routes...)
}

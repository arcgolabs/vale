package raftnode

import (
	"context"
	"encoding/json"
	"io"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	sm "github.com/lni/dragonboat/v3/statemachine"
	"github.com/samber/oops"
)

const lookupState = "state"

type fsm struct {
	group     string
	clusterID uint64
	state     State
	store     *stateStore
}

func newFSM(group string, clusterID uint64, store *stateStore) *fsm {
	if group == "" {
		group = groupNameFromClusterID(clusterID)
	}
	state := State{}
	if store != nil {
		if loaded, ok, err := store.LoadState(context.Background(), group); err == nil && ok {
			state = loaded
		}
	}
	return &fsm{
		group:     group,
		clusterID: clusterID,
		state:     cloneState(state),
		store:     store,
	}
}

func (f *fsm) Update(data []byte) (sm.Result, error) {
	state, err := applyCommand(f.state, data)
	if err != nil {
		return sm.Result{}, oops.
			In("raftnode").
			With("group", f.group, "cluster_id", f.clusterID, "bytes", len(data)).
			Wrapf(err, "apply raft fsm command")
	}
	if f.store != nil {
		if err := f.store.SaveState(context.Background(), f.group, state); err != nil {
			return sm.Result{}, oops.
				In("raftnode").
				With("group", f.group, "cluster_id", f.clusterID, "version", state.Version).
				Wrapf(err, "persist raft fsm state")
		}
	}
	f.state = state
	return sm.Result{Value: state.Version}, nil
}

func (f *fsm) Lookup(query any) (any, error) {
	if value, ok := query.(string); ok && value == lookupState {
		return cloneState(f.state), nil
	}
	return cloneState(f.state), nil
}

func (f *fsm) SaveSnapshot(writer io.Writer, _ sm.ISnapshotFileCollection, done <-chan struct{}) error {
	data, err := json.Marshal(f.state)
	if err != nil {
		return oops.
			In("raftnode").
			With("group", f.group, "cluster_id", f.clusterID).
			Wrapf(err, "marshal raft fsm snapshot")
	}
	select {
	case <-done:
		return sm.ErrSnapshotStopped
	default:
	}
	if _, err := writer.Write(data); err != nil {
		return oops.
			In("raftnode").
			With("group", f.group, "cluster_id", f.clusterID).
			Wrapf(err, "write raft fsm snapshot")
	}
	return nil
}

func (f *fsm) RecoverFromSnapshot(reader io.Reader, _ []sm.SnapshotFile, done <-chan struct{}) error {
	select {
	case <-done:
		return sm.ErrSnapshotStopped
	default:
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return oops.
			In("raftnode").
			With("group", f.group, "cluster_id", f.clusterID).
			Wrapf(err, "read raft fsm snapshot")
	}
	var state State
	if len(data) > 0 {
		if err := json.Unmarshal(data, &state); err != nil {
			return oops.
				In("raftnode").
				With("group", f.group, "cluster_id", f.clusterID).
				Wrapf(err, "unmarshal raft fsm snapshot")
		}
	}
	f.state = cloneState(state)
	if f.store != nil {
		if err := f.store.SaveState(context.Background(), f.group, f.state); err != nil {
			return oops.
				In("raftnode").
				With("group", f.group, "cluster_id", f.clusterID, "version", f.state.Version).
				Wrapf(err, "persist restored raft fsm state")
		}
	}
	return nil
}

func (f *fsm) Close() error {
	return nil
}

func applyCommand(current State, data []byte) (State, error) {
	if !json.Valid(data) {
		return State{
			Version:  current.Version + 1,
			Snapshot: cloneSnapshot(current.Snapshot),
			Routes:   cloneRoutes(current.Routes),
			Raw:      append(json.RawMessage(nil), data...),
		}, nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return current, oops.
			In("raftnode").
			Wrapf(err, "decode raft command envelope")
	}
	if envelope.Type == "" {
		return State{
			Version:  current.Version + 1,
			Snapshot: cloneSnapshot(current.Snapshot),
			Routes:   cloneRoutes(current.Routes),
			Raw:      append(json.RawMessage(nil), data...),
		}, nil
	}
	command := Command{}
	if err := json.Unmarshal(data, &command); err != nil {
		return current, oops.
			In("raftnode").
			With("command_type", envelope.Type).
			Wrapf(err, "decode raft command")
	}

	next := State{
		Version:   current.Version + 1,
		AppliedAt: time.Time{},
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
			With("command_type", command.Type).
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

func cloneRoutes(routes *collectionlist.List[RouteRecord]) *collectionlist.List[RouteRecord] {
	if routes == nil || routes.IsEmpty() {
		return nil
	}
	return routes.Clone()
}

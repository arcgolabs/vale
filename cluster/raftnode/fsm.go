package raftnode

import (
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
}

func newFSM(group string, clusterID uint64) *fsm {
	if group == "" {
		group = groupNameFromClusterID(clusterID)
	}
	return &fsm{
		group:     group,
		clusterID: clusterID,
		state:     State{},
	}
}

func (f *fsm) Update(data []byte) (sm.Result, error) {
	state, result, err := applyCommand(f.state, data)
	if err != nil {
		return sm.Result{}, oops.
			In("raftnode").
			With("group", f.group, "cluster_id", f.clusterID, "bytes", len(data)).
			Wrapf(err, "apply raft fsm command")
	}
	f.state = state
	if result.Value == 0 {
		result.Value = state.Version
	}
	return result, nil
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
	return nil
}

func (f *fsm) Close() error {
	return nil
}

func applyCommand(current State, data []byte) (State, sm.Result, error) {
	if !json.Valid(data) {
		return rawState(current, data), sm.Result{}, nil
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return current, sm.Result{}, oops.
			In("raftnode").
			Wrapf(err, "decode raft command envelope")
	}
	if envelope.Type == "" {
		return rawState(current, data), sm.Result{}, nil
	}
	command := Command{}
	if err := json.Unmarshal(data, &command); err != nil {
		return current, sm.Result{}, oops.
			In("raftnode").
			With("command_type", envelope.Type).
			Wrapf(err, "decode raft command")
	}

	return applyTypedCommand(current, nextState(current, command), command)
}

func rawState(current State, data []byte) State {
	return State{
		Version:      current.Version + 1,
		Snapshot:     cloneSnapshot(current.Snapshot),
		Routes:       cloneRoutes(current.Routes),
		Certificates: cloneCertificates(current.Certificates),
		Locks:        cloneCertificateLocks(current.Locks),
		Raw:          append(json.RawMessage(nil), data...),
	}
}

func nextState(current State, command Command) State {
	return State{
		Version:      current.Version + 1,
		AppliedAt:    time.Time{},
		Snapshot:     cloneSnapshot(current.Snapshot),
		Routes:       cloneRoutes(current.Routes),
		Certificates: cloneCertificates(current.Certificates),
		Locks:        cloneCertificateLocks(current.Locks),
		Raw:          append(json.RawMessage(nil), command.Raw...),
	}
}

func applyTypedCommand(current, next State, command Command) (State, sm.Result, error) {
	switch command.Type {
	case CommandTypeSnapshotUpdate:
		next.Snapshot = cloneSnapshot(command.Snapshot)
		return next, sm.Result{}, nil
	case CommandTypeRouteSync:
		next.Snapshot = cloneSnapshot(command.Snapshot)
		next.Routes = cloneRoutes(command.Routes)
		return next, sm.Result{}, nil
	case CommandTypeCertificateStore:
		return applyCertificateStore(next, command.Certificate)
	case CommandTypeCertificateDelete:
		return applyCertificateDelete(next, command.Certificate)
	case CommandTypeCertificateLockAcquire:
		return applyCertificateLockAcquire(next, command.Lock)
	case CommandTypeCertificateLockRelease:
		return applyCertificateLockRelease(next, command.Lock)
	default:
		return current, sm.Result{}, oops.
			In("raftnode").
			With("command_type", command.Type).
			Errorf("unsupported raft command type %q", command.Type)
	}
}

func cloneState(state State) State {
	return State{
		Version:      state.Version,
		AppliedAt:    state.AppliedAt,
		Snapshot:     cloneSnapshot(state.Snapshot),
		Routes:       cloneRoutes(state.Routes),
		Certificates: cloneCertificates(state.Certificates),
		Locks:        cloneCertificateLocks(state.Locks),
		Raw:          append(json.RawMessage(nil), state.Raw...),
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

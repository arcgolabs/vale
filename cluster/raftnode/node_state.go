package raftnode

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/samber/oops"
)

func (n *Node) AppliedState() State {
	return n.AppliedGroupState(DefaultGroupName)
}

func (n *Node) AppliedGroupState(group string) State {
	if !n.IsEnabled() {
		return State{}
	}
	raftGroup, ok := n.group(group)
	if !ok {
		return State{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := n.nodeHost.SyncRead(ctx, raftGroup.id, lookupState)
	if err != nil {
		if n.logger != nil {
			n.logger.Error("read applied raft state failed", "group", raftGroup.name, "error", err)
		}
		return State{}
	}
	state, ok := result.(State)
	if !ok {
		if n.logger != nil {
			n.logger.Error("read applied raft state returned unexpected type", "group", raftGroup.name, "type", stateType(result))
		}
		return State{}
	}
	return cloneState(state)
}

func (n *Node) AppliedGroupStateJSON(group string, timeout time.Duration) ([]byte, error) {
	if !n.IsEnabled() {
		return nil, ErrNotRunning
	}
	raftGroup, ok := n.group(group)
	if !ok {
		return nil, oops.
			In("raftnode").
			With("group", group).
			New("raft group is not configured")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	result, err := n.nodeHost.SyncRead(ctx, raftGroup.id, lookupState)
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("group", raftGroup.name, "timeout", timeout.String()).
			Wrapf(err, "read applied raft state")
	}
	state, ok := result.(State)
	if !ok {
		return nil, oops.
			In("raftnode").
			With("group", raftGroup.name, "type", stateType(result)).
			New("read applied raft state returned unexpected type")
	}
	data, err := json.Marshal(cloneState(state))
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("group", raftGroup.name).
			Wrapf(err, "marshal applied raft state")
	}
	return data, nil
}

func stateType(value any) string {
	if value == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", value)
}

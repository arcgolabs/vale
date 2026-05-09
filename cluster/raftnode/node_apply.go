package raftnode

import (
	"context"
	"time"

	"github.com/samber/oops"
)

func (n *Node) Apply(data []byte, timeout time.Duration) error {
	return n.ApplyGroup(DefaultGroupName, data, timeout)
}

func (n *Node) ApplyGroup(group string, data []byte, timeout time.Duration) error {
	_, err := n.ProposeGroup(group, data, timeout)
	return err
}

func (n *Node) ProposeGroup(group string, data []byte, timeout time.Duration) ([]byte, error) {
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if n.logger != nil {
		n.logger.Info("raft apply started", "group", raftGroup.name, "bytes", len(data), "timeout", timeout)
	}
	session := n.nodeHost.GetNoOPSession(raftGroup.id)
	result, err := n.nodeHost.SyncPropose(ctx, session, data)
	if err != nil && n.logger != nil {
		n.logger.Error("raft apply failed", "group", raftGroup.name, "error", err)
	}
	if err != nil {
		return nil, oops.
			In("raftnode").
			With("group", raftGroup.name, "bytes", len(data), "timeout", timeout.String()).
			Wrapf(err, "apply raft command")
	}
	return result.Data, nil
}

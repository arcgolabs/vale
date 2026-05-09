package raftnode

import (
	"errors"

	"github.com/samber/oops"
)

func (n *Node) Shutdown() error {
	if !n.IsEnabled() {
		return nil
	}
	if n.logger != nil {
		n.logger.Info("raft shutdown started")
	}
	stopErr := n.stopGroups()
	if n.ownsNodeHost {
		n.nodeHost.Stop()
	}
	n.nodeHost = nil
	return stopErr
}

func (n *Node) stopGroups() error {
	if n == nil || n.nodeHost == nil || n.groups == nil {
		return nil
	}
	var stopErr error
	n.groups.Range(func(_ string, group *raftGroup) bool {
		if group == nil {
			return true
		}
		if err := n.nodeHost.StopCluster(group.id); err != nil {
			stopErr = errors.Join(stopErr, oops.
				In("raftnode").
				With("group", group.name, "cluster_id", group.id).
				Wrapf(err, "stop raft group"))
		}
		return true
	})
	return stopErr
}

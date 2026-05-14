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
	discoveryErr := n.stopDiscovery()
	stopErr := n.stopGroups()
	if n.ownsNodeHost {
		n.nodeHost.Stop()
	}
	n.nodeHost = nil
	return errors.Join(discoveryErr, stopErr)
}

func (n *Node) stopDiscovery() error {
	if n == nil || n.discovery == nil {
		return nil
	}
	if n.discoveryCancel != nil {
		n.discoveryCancel()
	}
	if n.discoveryDone != nil {
		<-n.discoveryDone
	}
	err := n.discovery.Shutdown()
	n.discovery = nil
	n.discoveryCancel = nil
	n.discoveryDone = nil
	n.discoveryChanged = nil
	if err != nil {
		return oops.
			In("raftnode").
			Wrapf(err, "stop discovery")
	}
	return nil
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

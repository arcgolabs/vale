package raftnode

import (
	"context"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

type Discovery interface {
	Start(ctx context.Context, local DiscoveryNode, onChange func()) error
	Peers() *collectionlist.List[DiscoveryNode]
	Shutdown() error
}

type DiscoveryNode struct {
	ID           string
	RaftAddress  string
	DeploymentID uint64
	Groups       *collectionlist.List[string]
}

func (n DiscoveryNode) normalized() DiscoveryNode {
	n.ID = strings.TrimSpace(n.ID)
	n.RaftAddress = strings.TrimSpace(n.RaftAddress)
	if n.Groups == nil {
		n.Groups = collectionlist.NewList[string]()
	}
	return n
}

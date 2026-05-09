package gateway

import (
	"log/slog"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

const ClusterGroupRoutes = "routes"

type ClusterPeer = mapping.Map[string, string]

type ClusterFactory func(*slog.Logger) (Cluster, error)

type Cluster interface {
	IsLeader() bool
	Apply([]byte, time.Duration) error
	Status() *mapping.Map[string, any]
	Peers() (*collectionlist.List[*ClusterPeer], error)
	AddVoter(id string, address string, timeout time.Duration) error
	RemoveServer(id string, timeout time.Duration) error
	Shutdown() error
}

type GroupCluster interface {
	IsGroupLeader(group string) bool
	ApplyGroup(group string, data []byte, timeout time.Duration) error
	GroupPeers(group string) (*collectionlist.List[*ClusterPeer], error)
	AddGroupVoter(group string, id string, address string, timeout time.Duration) error
	RemoveGroupServer(group string, id string, timeout time.Duration) error
}

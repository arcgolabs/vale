package gateway

import (
	"log/slog"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type ClusterFactory func(*slog.Logger) (Cluster, error)

type Cluster interface {
	IsLeader() bool
	Apply([]byte, time.Duration) error
	Status() *mapping.Map[string, any]
	Peers() (*collectionlist.List[ClusterPeer], error)
	AddVoter(id string, address string, timeout time.Duration) error
	RemoveServer(id string, timeout time.Duration) error
	Shutdown() error
}

type ClusterPeer struct {
	ID       string `json:"id"`
	Address  string `json:"address"`
	Suffrage string `json:"suffrage"`
}

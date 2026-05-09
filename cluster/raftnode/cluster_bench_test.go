package raftnode_test

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/cluster/raftnode"
)

func BenchmarkDragonboatClusterRouteSync(b *testing.B) {
	for _, groupCount := range []int{1, 8} {
		b.Run(fmt.Sprintf("groups_%d", groupCount), func(b *testing.B) {
			cluster := newBenchmarkCluster(b, groupCount)
			payload := []byte(`{"type":"route_sync","snapshot":{"built_at":"2026-05-07T00:00:00Z","services":1,"routes":1,"proxy_engine":"oxy"},"routes":[{"name":"api","entrypoint":"web","path_prefix":"/api","service":"svc"}]}`)

			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				group, _ := cluster.groups.Get(i % cluster.groups.Len())
				leader, _ := cluster.leaders.Get(group)
				if err := leader.ApplyGroup(group, payload, 5*time.Second); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

type benchmarkCluster struct {
	nodes   *collectionlist.List[*raftnode.Node]
	groups  *collectionlist.List[string]
	leaders *mapping.Map[string, *raftnode.Node]
}

func newBenchmarkCluster(b *testing.B, groupCount int) benchmarkCluster {
	b.Helper()

	quietDragonboatLogs()
	addresses := collectionlist.NewListWithCapacity[string](3)
	for range 3 {
		addresses.Add(freeAddrForBenchmark(b))
	}
	groups := benchmarkGroups(groupCount, addresses)
	nodes := collectionlist.NewListWithCapacity[*raftnode.Node](3)
	addresses.Range(func(index int, address string) bool {
		node, err := raftnode.New(raftnode.Config{
			Enabled:   true,
			NodeID:    fmt.Sprintf("node-%d", index+1),
			BindAddr:  address,
			DataDir:   filepath.Join(b.TempDir(), fmt.Sprintf("node-%d", index+1)),
			Bootstrap: false,
			Groups:    groups,
		}, discardLogger())
		if err != nil {
			b.Fatal(err)
		}
		nodes.Add(node)
		return true
	})
	b.Cleanup(func() {
		nodes.Range(func(_ int, node *raftnode.Node) bool {
			if err := node.Shutdown(); err != nil {
				b.Fatal(err)
			}
			return true
		})
	})

	groupNames := benchmarkGroupNames(groupCount)
	leaders := mapping.NewMapWithCapacity[string, *raftnode.Node](groupNames.Len())
	groupNames.Range(func(_ int, group string) bool {
		leaders.Set(group, waitBenchmarkGroupLeader(b, nodes, group))
		return true
	})
	return benchmarkCluster{
		nodes:   nodes,
		groups:  groupNames,
		leaders: leaders,
	}
}

func benchmarkGroups(groupCount int, addresses *collectionlist.List[string]) *collectionlist.List[raftnode.GroupConfig] {
	groups := collectionlist.NewListWithCapacity[raftnode.GroupConfig](groupCount)
	nextID := uint64(1)
	benchmarkGroupNames(groupCount).Range(func(_ int, group string) bool {
		groups.Add(raftnode.GroupConfig{
			Name:           group,
			ID:             nextID,
			InitialMembers: benchmarkMembers(addresses),
		})
		nextID++
		return true
	})
	return groups
}

func benchmarkGroupNames(groupCount int) *collectionlist.List[string] {
	groups := collectionlist.NewListWithCapacity[string](groupCount)
	for index := range groupCount {
		if index == 0 {
			groups.Add(raftnode.DefaultGroupName)
			continue
		}
		groups.Add(fmt.Sprintf("routes-%d", index+1))
	}
	return groups
}

func benchmarkMembers(addresses *collectionlist.List[string]) *mapping.Map[string, string] {
	members := mapping.NewMapWithCapacity[string, string](addresses.Len())
	addresses.Range(func(index int, address string) bool {
		members.Set(fmt.Sprintf("node-%d", index+1), address)
		return true
	})
	return members
}

func waitBenchmarkGroupLeader(b *testing.B, nodes *collectionlist.List[*raftnode.Node], group string) *raftnode.Node {
	b.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var leader *raftnode.Node
		nodes.Range(func(_ int, node *raftnode.Node) bool {
			if node.IsGroupLeader(group) {
				leader = node
				return false
			}
			return true
		})
		if leader != nil {
			return leader
		}
		time.Sleep(20 * time.Millisecond)
	}
	b.Fatalf("raft group %q did not elect a leader", group)
	return nil
}

func freeAddrForBenchmark(b *testing.B) string {
	b.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(b.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := listener.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	return listener.Addr().String()
}

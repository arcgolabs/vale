package raftnode

import (
	"encoding/json"
	"log/slog"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/hashicorp/memberlist"
)

func (d *MemberlistDiscovery) NodeMeta(limit int) []byte {
	data, err := json.Marshal(discoveryNodeMeta{
		ID:           d.local.ID,
		RaftAddress:  d.local.RaftAddress,
		DeploymentID: d.local.DeploymentID,
		Groups:       discoveryGroups(d.local.Groups),
	})
	if err != nil || len(data) > limit {
		return nil
	}
	return data
}

func discoveryGroups(groups *collectionlist.List[string]) []string {
	if groups == nil {
		return nil
	}
	return groups.Values()
}

func (d *MemberlistDiscovery) NotifyMsg([]byte) {}

func (d *MemberlistDiscovery) GetBroadcasts(_, _ int) [][]byte {
	return nil
}

func (d *MemberlistDiscovery) LocalState(bool) []byte {
	return d.NodeMeta(memberlist.MetaMaxSize)
}

func (d *MemberlistDiscovery) MergeRemoteState([]byte, bool) {}

func (d *MemberlistDiscovery) NotifyJoin(node *memberlist.Node) {
	d.upsertMember(node)
}

func (d *MemberlistDiscovery) NotifyLeave(node *memberlist.Node) {
	d.removeMember(node)
}

func (d *MemberlistDiscovery) NotifyUpdate(node *memberlist.Node) {
	d.upsertMember(node)
}

type memberlistSlogWriter struct {
	logger *slog.Logger
}

func (w memberlistSlogWriter) Write(data []byte) (int, error) {
	if w.logger != nil {
		message := strings.TrimSpace(string(data))
		if message != "" {
			w.logger.Debug("memberlist", "message", message)
		}
	}
	return len(data), nil
}

package gateway

import (
	"context"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/httpx"
	"github.com/arcgolabs/httpx/adapter"
	httpxstd "github.com/arcgolabs/httpx/adapter/std"
	"github.com/arcgolabs/vale/runtime"
	"github.com/go-chi/chi/v5"
	"github.com/samber/oops"
)

type adminRoutesInput struct {
	Entrypoint string `query:"entrypoint"`
	Service    string `query:"service"`
	Host       string `query:"host"`
	PathPrefix string `query:"path_prefix"`
}

type adminRoutesOutput struct {
	Body []runtime.RouteView `json:"body"`
}

type adminServicesOutput struct {
	Body []adminServiceView `json:"body"`
}

type adminEndpointsOutput struct {
	Body []runtime.EndpointView `json:"body"`
}

type adminReloadStatusOutput struct {
	Body ReloadStatusView `json:"body"`
}

type adminClusterStatusOutput struct {
	Body map[string]any `json:"body"`
}

type adminClusterPeersInput struct {
	Group string `query:"group"`
}

type adminClusterPeersOutput struct {
	Body []map[string]string `json:"body"`
}

type adminJoinInput struct {
	Body adminJoinRequest `json:"body"`
}

type adminJoinRequest struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Group   string `json:"group,omitempty"`
}

type adminLeaveInput struct {
	Body adminLeaveRequest `json:"body"`
}

type adminLeaveRequest struct {
	ID    string `json:"id"`
	Group string `json:"group,omitempty"`
}

type adminOKOutput struct {
	Body map[string]string `json:"body"`
}

func (g *Gateway) buildAdminMux() http.Handler {
	router := chi.NewMux()
	router.Handle("/metrics", g.runtime.MetricsHandler())

	adminAdapter := httpxstd.New(router, adapter.HumaOptions{
		Title:             "Vale Admin API",
		Version:           "0.1.0",
		Description:       "Runtime and cluster control-plane endpoints.",
		DisableDocsRoutes: true,
	})
	adminServer := httpx.New(
		httpx.WithAdapter(adminAdapter),
		httpx.WithLogger(g.logger),
		httpx.WithValidation(),
	)
	g.registerAdminRoutes(adminServer)
	return router
}

func (g *Gateway) registerAdminRoutes(server httpx.ServerRuntime) {
	httpx.MustGet(server, "/admin/routes", g.handleAdminRoutes)
	httpx.MustGet(server, "/admin/services", g.handleAdminServices)
	httpx.MustGet(server, "/admin/endpoints", g.handleAdminEndpoints)
	httpx.MustGet(server, "/admin/reload/status", g.handleAdminReloadStatus)
	httpx.MustGet(server, "/admin/cluster/status", g.handleAdminClusterStatus)
	httpx.MustGet(server, "/admin/cluster/peers", g.handleAdminClusterPeers)
	httpx.MustPost(server, "/admin/cluster/join", g.handleAdminClusterJoin)
	httpx.MustPost(server, "/admin/cluster/leave", g.handleAdminClusterLeave)
}

func (g *Gateway) handleAdminRoutes(_ context.Context, input *adminRoutesInput) (*adminRoutesOutput, error) {
	snapshot, err := g.adminSnapshot()
	if err != nil {
		return nil, err
	}
	return &adminRoutesOutput{Body: adminRoutesView(snapshot, runtime.RouteFilter{
		Entrypoint: input.Entrypoint,
		Service:    input.Service,
		Host:       input.Host,
		PathPrefix: input.PathPrefix,
	})}, nil
}

func (g *Gateway) handleAdminServices(context.Context, *struct{}) (*adminServicesOutput, error) {
	snapshot, err := g.adminSnapshot()
	if err != nil {
		return nil, err
	}
	return &adminServicesOutput{Body: adminServicesView(snapshot)}, nil
}

func (g *Gateway) handleAdminEndpoints(context.Context, *struct{}) (*adminEndpointsOutput, error) {
	snapshot, err := g.adminSnapshot()
	if err != nil {
		return nil, err
	}
	return &adminEndpointsOutput{Body: adminEndpointsView(snapshot)}, nil
}

func (g *Gateway) handleAdminReloadStatus(context.Context, *struct{}) (*adminReloadStatusOutput, error) {
	return &adminReloadStatusOutput{Body: g.reloadStatus()}, nil
}

func (g *Gateway) handleAdminClusterStatus(context.Context, *struct{}) (*adminClusterStatusOutput, error) {
	if g.cluster == nil {
		return &adminClusterStatusOutput{Body: adminAnyMapView(disabledClusterStatus())}, nil
	}
	return &adminClusterStatusOutput{Body: adminAnyMapView(g.cluster.Status())}, nil
}

func (g *Gateway) handleAdminClusterPeers(_ context.Context, input *adminClusterPeersInput) (*adminClusterPeersOutput, error) {
	if g.cluster == nil {
		return &adminClusterPeersOutput{Body: []map[string]string{}}, nil
	}
	peers, err := g.clusterPeers(input.Group)
	if err != nil {
		return nil, httpx.NewError(http.StatusInternalServerError, err.Error(), err)
	}
	return &adminClusterPeersOutput{Body: adminPeersView(peers)}, nil
}

func (g *Gateway) handleAdminClusterJoin(_ context.Context, input *adminJoinInput) (*adminOKOutput, error) {
	if err := g.requireClusterLeader(input.Body.Group, "join peers"); err != nil {
		return nil, err
	}
	if err := g.addClusterVoter(input.Body.Group, input.Body.ID, input.Body.Address, 5*time.Second); err != nil {
		return nil, httpx.NewError(http.StatusBadRequest, err.Error(), err)
	}
	return adminOK(), nil
}

func (g *Gateway) handleAdminClusterLeave(_ context.Context, input *adminLeaveInput) (*adminOKOutput, error) {
	if err := g.requireClusterLeader(input.Body.Group, "remove peers"); err != nil {
		return nil, err
	}
	if err := g.removeClusterServer(input.Body.Group, input.Body.ID, 5*time.Second); err != nil {
		return nil, httpx.NewError(http.StatusBadRequest, err.Error(), err)
	}
	return adminOK(), nil
}

func (g *Gateway) adminSnapshot() (*runtime.CompiledSnapshot, error) {
	if g.runtime == nil {
		return nil, httpx.NewError(http.StatusServiceUnavailable, "runtime not ready")
	}
	snapshot := g.runtime.Snapshot()
	if snapshot == nil {
		return nil, httpx.NewError(http.StatusServiceUnavailable, "runtime not ready")
	}
	return snapshot, nil
}

func (g *Gateway) requireClusterLeader(group, action string) error {
	if g.cluster == nil {
		return httpx.NewError(http.StatusBadRequest, "cluster is not configured")
	}
	if !g.clusterLeader(group) {
		return httpx.NewError(http.StatusConflict, "only leader can "+action)
	}
	return nil
}

func (g *Gateway) clusterPeers(group string) (*collectionlist.List[*ClusterPeer], error) {
	if groupCluster, ok := g.cluster.(GroupCluster); ok && group != "" {
		peers, err := groupCluster.GroupPeers(group)
		if err != nil {
			return nil, oops.
				In("gateway").
				With("group", group).
				Wrapf(err, "get raft group peers")
		}
		return peers, nil
	}
	peers, err := g.cluster.Peers()
	if err != nil {
		return nil, oops.
			In("gateway").
			Wrapf(err, "get raft peers")
	}
	return peers, nil
}

func (g *Gateway) addClusterVoter(group, id, address string, timeout time.Duration) error {
	if groupCluster, ok := g.cluster.(GroupCluster); ok && group != "" {
		if err := groupCluster.AddGroupVoter(group, id, address, timeout); err != nil {
			return oops.
				In("gateway").
				With("group", group, "id", id, "address", address).
				Wrapf(err, "add raft group voter")
		}
		return nil
	}
	if err := g.cluster.AddVoter(id, address, timeout); err != nil {
		return oops.
			In("gateway").
			With("id", id, "address", address).
			Wrapf(err, "add raft voter")
	}
	return nil
}

func (g *Gateway) removeClusterServer(group, id string, timeout time.Duration) error {
	if groupCluster, ok := g.cluster.(GroupCluster); ok && group != "" {
		if err := groupCluster.RemoveGroupServer(group, id, timeout); err != nil {
			return oops.
				In("gateway").
				With("group", group, "id", id).
				Wrapf(err, "remove raft group server")
		}
		return nil
	}
	if err := g.cluster.RemoveServer(id, timeout); err != nil {
		return oops.
			In("gateway").
			With("id", id).
			Wrapf(err, "remove raft server")
	}
	return nil
}

func (g *Gateway) clusterLeader(group string) bool {
	if groupCluster, ok := g.cluster.(GroupCluster); ok && group != "" {
		return groupCluster.IsGroupLeader(group)
	}
	return g.cluster.IsLeader()
}

func adminOK() *adminOKOutput {
	body := map[string]string{"status": "ok"}
	return &adminOKOutput{Body: body}
}

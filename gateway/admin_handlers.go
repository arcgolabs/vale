package gateway

import (
	"context"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/httpx"
	"github.com/arcgolabs/httpx/adapter"
	httpxstd "github.com/arcgolabs/httpx/adapter/std"
	"github.com/arcgolabs/vale/runtime"
	"github.com/go-chi/chi/v5"
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

type adminClusterStatusOutput struct {
	Body *mapping.Map[string, any] `json:"body"`
}

type adminClusterPeersOutput struct {
	Body *collectionlist.List[ClusterPeer] `json:"body"`
}

type adminJoinInput struct {
	Body adminJoinRequest `json:"body"`
}

type adminJoinRequest struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type adminLeaveInput struct {
	Body adminLeaveRequest `json:"body"`
}

type adminLeaveRequest struct {
	ID string `json:"id"`
}

type adminOKOutput struct {
	Body *mapping.Map[string, string] `json:"body"`
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

func (g *Gateway) handleAdminClusterStatus(context.Context, *struct{}) (*adminClusterStatusOutput, error) {
	if g.cluster == nil {
		return &adminClusterStatusOutput{Body: disabledClusterStatus()}, nil
	}
	return &adminClusterStatusOutput{Body: g.cluster.Status()}, nil
}

func (g *Gateway) handleAdminClusterPeers(context.Context, *struct{}) (*adminClusterPeersOutput, error) {
	if g.cluster == nil {
		return &adminClusterPeersOutput{Body: collectionlist.NewList[ClusterPeer]()}, nil
	}
	peers, err := g.cluster.Peers()
	if err != nil {
		return nil, httpx.NewError(http.StatusInternalServerError, err.Error(), err)
	}
	return &adminClusterPeersOutput{Body: peers}, nil
}

func (g *Gateway) handleAdminClusterJoin(_ context.Context, input *adminJoinInput) (*adminOKOutput, error) {
	if err := g.requireClusterLeader("join peers"); err != nil {
		return nil, err
	}
	if err := g.cluster.AddVoter(input.Body.ID, input.Body.Address, 5*time.Second); err != nil {
		return nil, httpx.NewError(http.StatusBadRequest, err.Error(), err)
	}
	return adminOK(), nil
}

func (g *Gateway) handleAdminClusterLeave(_ context.Context, input *adminLeaveInput) (*adminOKOutput, error) {
	if err := g.requireClusterLeader("remove peers"); err != nil {
		return nil, err
	}
	if err := g.cluster.RemoveServer(input.Body.ID, 5*time.Second); err != nil {
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

func (g *Gateway) requireClusterLeader(action string) error {
	if g.cluster == nil {
		return httpx.NewError(http.StatusBadRequest, "raft is not enabled")
	}
	if !g.cluster.IsLeader() {
		return httpx.NewError(http.StatusConflict, "only leader can "+action)
	}
	return nil
}

func adminOK() *adminOKOutput {
	body := mapping.NewMap[string, string]()
	body.Set("status", "ok")
	return &adminOKOutput{Body: body}
}

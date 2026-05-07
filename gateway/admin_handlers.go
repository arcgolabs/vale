package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/arcgolabs/vela/runtime"
)

type adminJoinRequest struct {
	ID      string `json:"id"`
	Address string `json:"address"`
}

type adminLeaveRequest struct {
	ID string `json:"id"`
}

func (g *Gateway) buildAdminMux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", g.runtime.MetricsHandler())
	mux.HandleFunc("/admin/routes", g.handleAdminRoutes)
	mux.HandleFunc("/admin/services", g.handleAdminServices)
	mux.HandleFunc("/admin/endpoints", g.handleAdminEndpoints)
	mux.HandleFunc("/admin/cluster/status", g.handleAdminClusterStatus)
	mux.HandleFunc("/admin/cluster/peers", g.handleAdminClusterPeers)
	mux.HandleFunc("/admin/cluster/join", g.handleAdminClusterJoin)
	mux.HandleFunc("/admin/cluster/leave", g.handleAdminClusterLeave)
	return mux
}

func (g *Gateway) handleAdminRoutes(w http.ResponseWriter, r *http.Request) {
	snapshot, ok := g.adminSnapshot(w)
	if !ok {
		return
	}
	query := r.URL.Query()
	writeJSON(w, http.StatusOK, adminRoutesView(snapshot, runtime.RouteFilter{
		Entrypoint: query.Get("entrypoint"),
		Service:    query.Get("service"),
		Host:       query.Get("host"),
		PathPrefix: query.Get("path_prefix"),
	}))
}

func (g *Gateway) handleAdminServices(w http.ResponseWriter, _ *http.Request) {
	snapshot, ok := g.adminSnapshot(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, adminServicesView(snapshot))
}

func (g *Gateway) handleAdminEndpoints(w http.ResponseWriter, _ *http.Request) {
	snapshot, ok := g.adminSnapshot(w)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, adminEndpointsView(snapshot))
}

func (g *Gateway) handleAdminClusterStatus(w http.ResponseWriter, _ *http.Request) {
	if g.cluster == nil {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, g.cluster.Status())
}

func (g *Gateway) handleAdminClusterPeers(w http.ResponseWriter, _ *http.Request) {
	if g.cluster == nil {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	peers, err := g.cluster.Peers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, peers)
}

func (g *Gateway) handleAdminClusterJoin(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) || !g.requireClusterLeader(w, "join peers") {
		return
	}
	req, ok := decodeAdminJSON[adminJoinRequest](w, r)
	if !ok {
		return
	}
	if err := g.cluster.AddVoter(req.ID, req.Address, 5*time.Second); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) handleAdminClusterLeave(w http.ResponseWriter, r *http.Request) {
	if !requirePost(w, r) || !g.requireClusterLeader(w, "remove peers") {
		return
	}
	req, ok := decodeAdminJSON[adminLeaveRequest](w, r)
	if !ok {
		return
	}
	if err := g.cluster.RemoveServer(req.ID, 5*time.Second); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (g *Gateway) adminSnapshot(w http.ResponseWriter) (*runtime.CompiledSnapshot, bool) {
	if g.runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not ready"})
		return nil, false
	}
	snapshot := g.runtime.Snapshot()
	if snapshot == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not ready"})
		return nil, false
	}
	return snapshot, true
}

func (g *Gateway) requireClusterLeader(w http.ResponseWriter, action string) bool {
	if g.cluster == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "raft is not enabled"})
		return false
	}
	if !g.cluster.IsLeader() {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "only leader can " + action})
		return false
	}
	return true
}

func requirePost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodPost {
		return true
	}
	w.Header().Set("Allow", http.MethodPost)
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	return false
}

func decodeAdminJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return req, false
	}
	return req, true
}

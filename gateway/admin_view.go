package gateway

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/runtime"
)

type adminServiceView struct {
	Name      string                 `json:"name"`
	Strategy  string                 `json:"strategy"`
	Endpoints []runtime.EndpointView `json:"endpoints"`
}

func adminRoutesView(snapshot *runtime.CompiledSnapshot, filter runtime.RouteFilter) []runtime.RouteView {
	if snapshot == nil {
		return nil
	}
	return snapshot.QueryRoutes(filter).Values()
}

func adminServicesView(snapshot *runtime.CompiledSnapshot) []adminServiceView {
	if snapshot == nil {
		return nil
	}
	return collectionlist.MapList(snapshot.ServicesView(), func(_ int, service runtime.ServiceView) adminServiceView {
		return adminServiceView{
			Name:      service.Name,
			Strategy:  service.Strategy,
			Endpoints: service.Endpoints.Values(),
		}
	}).Values()
}

func adminEndpointsView(snapshot *runtime.CompiledSnapshot) []runtime.EndpointView {
	if snapshot == nil {
		return nil
	}
	return collectionlist.FlatMapList(snapshot.ServicesView(), func(_ int, service runtime.ServiceView) []runtime.EndpointView {
		return service.Endpoints.Values()
	}).Values()
}

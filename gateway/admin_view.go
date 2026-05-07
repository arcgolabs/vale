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
	services := collectionlist.NewListWithCapacity[adminServiceView](snapshot.ServicesView().Len())
	snapshot.ServicesView().Range(func(_ int, service runtime.ServiceView) bool {
		services.Add(adminServiceView{
			Name:      service.Name,
			Strategy:  service.Strategy,
			Endpoints: service.Endpoints.Values(),
		})
		return true
	})
	return services.Values()
}

func adminEndpointsView(snapshot *runtime.CompiledSnapshot) []runtime.EndpointView {
	if snapshot == nil {
		return nil
	}
	endpoints := collectionlist.NewList[runtime.EndpointView]()
	snapshot.ServicesView().Range(func(_ int, service runtime.ServiceView) bool {
		endpoints.Merge(service.Endpoints)
		return true
	})
	return endpoints.Values()
}

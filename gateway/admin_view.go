package gateway

import (
	"iter"

	"github.com/arcgolabs/vale/runtime"
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
	return snapshot.ServicesView().Stream().
		Map(func(service runtime.ServiceView) adminServiceView {
			return adminServiceView{
				Name:      service.Name,
				Strategy:  service.Strategy,
				Endpoints: service.Endpoints.Values(),
			}
		}).
		ToSlice()
}

func adminEndpointsView(snapshot *runtime.CompiledSnapshot) []runtime.EndpointView {
	if snapshot == nil {
		return nil
	}
	return snapshot.ServicesView().Stream().
		FlatMap(func(service runtime.ServiceView) iter.Seq[runtime.EndpointView] {
			return service.Endpoints.Stream().Values()
		}).
		ToSlice()
}

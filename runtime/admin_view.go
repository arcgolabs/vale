package runtime

import collectionlist "github.com/arcgolabs/collectionx/list"

type RouteView struct {
	Name       string `json:"name"`
	Entrypoint string `json:"entrypoint"`
	Host       string `json:"host,omitempty"`
	PathPrefix string `json:"path_prefix,omitempty"`
	Method     string `json:"method,omitempty"`
	Service    string `json:"service"`
}

type EndpointView struct {
	URL         string `json:"url"`
	Weight      int    `json:"weight"`
	Healthy     bool   `json:"healthy"`
	LastChecked int64  `json:"last_checked"`
}

type ServiceView struct {
	Name      string         `json:"name"`
	Strategy  string         `json:"strategy"`
	Endpoints []EndpointView `json:"endpoints"`
}

func (s *CompiledSnapshot) Routes() []RouteView {
	routeList := collectionlist.NewList[RouteView]()
	for entrypoint, routes := range s.RoutesByEntrypoint {
		for _, route := range routes {
			routeList.Add(RouteView{
				Name:       route.Name,
				Entrypoint: entrypoint,
				Host:       route.Host,
				PathPrefix: route.PathPrefix,
				Method:     route.Method,
				Service:    route.Service.Name,
			})
		}
	}
	return routeList.Values()
}

func (s *CompiledSnapshot) ServicesView() []ServiceView {
	serviceList := collectionlist.NewListWithCapacity[ServiceView](len(s.Services))
	for _, service := range s.Services {
		endpointList := collectionlist.NewListWithCapacity[EndpointView](len(service.Endpoints))
		for _, endpoint := range service.Endpoints {
			endpointList.Add(EndpointView{
				URL:         endpoint.URL.String(),
				Weight:      endpoint.Weight,
				Healthy:     endpoint.Healthy.Load(),
				LastChecked: endpoint.LastChecked.Load(),
			})
		}
		serviceView := ServiceView{
			Name:      service.Name,
			Strategy:  service.Strategy,
			Endpoints: endpointList.Values(),
		}
		serviceList.Add(serviceView)
	}
	return serviceList.Values()
}

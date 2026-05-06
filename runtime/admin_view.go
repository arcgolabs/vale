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
	Name      string                             `json:"name"`
	Strategy  string                             `json:"strategy"`
	Endpoints *collectionlist.List[EndpointView] `json:"endpoints"`
}

func (s *CompiledSnapshot) Routes() *collectionlist.List[RouteView] {
	routeList := collectionlist.NewList[RouteView]()
	if s == nil || s.RoutesByEntrypoint == nil {
		return routeList
	}
	s.RoutesByEntrypoint.Range(func(entrypoint string, routes []*CompiledRoute) bool {
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
		return true
	})
	return routeList
}

func (s *CompiledSnapshot) ServicesView() *collectionlist.List[ServiceView] {
	if s == nil || s.Services == nil {
		return collectionlist.NewList[ServiceView]()
	}
	serviceList := collectionlist.NewListWithCapacity[ServiceView](s.Services.Len())
	s.Services.Range(func(_ string, service *ServiceRuntime) bool {
		endpointList := collectionlist.NewListWithCapacity[EndpointView](service.Endpoints.Len())
		service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
			endpointList.Add(EndpointView{
				URL:         endpoint.URL.String(),
				Weight:      endpoint.Weight,
				Healthy:     endpoint.Healthy.Load(),
				LastChecked: endpoint.LastChecked.Load(),
			})
			return true
		})
		serviceView := ServiceView{
			Name:      service.Name,
			Strategy:  service.Strategy,
			Endpoints: endpointList,
		}
		serviceList.Add(serviceView)
		return true
	})
	return serviceList
}

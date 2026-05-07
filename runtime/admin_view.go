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
	return s.QueryRoutes(RouteFilter{})
}

func (s *CompiledSnapshot) ServicesView() *collectionlist.List[ServiceView] {
	if s == nil || s.Services == nil {
		return collectionlist.NewList[ServiceView]()
	}
	serviceList := collectionlist.NewListWithCapacity[ServiceView](s.Services.Len())
	s.Services.Range(func(_ string, service *ServiceRuntime) bool {
		serviceView := ServiceView{
			Name:      service.Name,
			Strategy:  service.Strategy,
			Endpoints: endpointViews(service),
		}
		serviceList.Add(serviceView)
		return true
	})
	return serviceList
}

func endpointViews(service *ServiceRuntime) *collectionlist.List[EndpointView] {
	if service == nil {
		return collectionlist.NewList[EndpointView]()
	}
	return collectionlist.MapList(service.Endpoints, func(_ int, endpoint *EndpointRuntime) EndpointView {
		return EndpointView{
			URL:         endpoint.URL.String(),
			Weight:      endpoint.Weight,
			Healthy:     endpoint.Healthy.Load(),
			LastChecked: endpoint.LastChecked.Load(),
		}
	})
}

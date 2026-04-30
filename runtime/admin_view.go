package runtime

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
	result := make([]RouteView, 0)
	for entrypoint, routes := range s.RoutesByEntrypoint {
		for _, route := range routes {
			result = append(result, RouteView{
				Name:       route.Name,
				Entrypoint: entrypoint,
				Host:       route.Host,
				PathPrefix: route.PathPrefix,
				Method:     route.Method,
				Service:    route.Service.Name,
			})
		}
	}
	return result
}

func (s *CompiledSnapshot) ServicesView() []ServiceView {
	result := make([]ServiceView, 0, len(s.Services))
	for _, service := range s.Services {
		serviceView := ServiceView{
			Name:      service.Name,
			Strategy:  service.Strategy,
			Endpoints: make([]EndpointView, 0, len(service.Endpoints)),
		}
		for _, endpoint := range service.Endpoints {
			serviceView.Endpoints = append(serviceView.Endpoints, EndpointView{
				URL:         endpoint.URL.String(),
				Weight:      endpoint.Weight,
				Healthy:     endpoint.Healthy.Load(),
				LastChecked: endpoint.LastChecked.Load(),
			})
		}
		result = append(result, serviceView)
	}
	return result
}

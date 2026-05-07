package runtime

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

func NewSnapshot() *CompiledSnapshot {
	return &CompiledSnapshot{
		Entrypoints:        mapping.NewMap[string, string](),
		EntrypointConfigs:  mapping.NewMap[string, EntrypointRuntime](),
		RoutesByEntrypoint: mapping.NewMultiMap[string, *CompiledRoute](),
		EntrypointMatchers: mapping.NewMap[string, *EntrypointMatcher](),
		Services:           mapping.NewMap[string, *ServiceRuntime](),
	}
}

func (s *CompiledSnapshot) BuildMatchers() *CompiledSnapshot {
	if s == nil {
		return nil
	}
	if s.EntrypointMatchers == nil {
		s.EntrypointMatchers = mapping.NewMap[string, *EntrypointMatcher]()
	}
	s.RoutesByEntrypoint.Range(func(entrypoint string, routes []*CompiledRoute) bool {
		s.EntrypointMatchers.Set(entrypoint, BuildEntrypointMatcher(routes))
		return true
	})
	return s
}

func (s *CompiledSnapshot) AddEntrypoint(name string, address string, entrypoint EntrypointRuntime) *CompiledSnapshot {
	if s == nil {
		return nil
	}
	if s.Entrypoints == nil {
		s.Entrypoints = mapping.NewMap[string, string]()
	}
	if s.EntrypointConfigs == nil {
		s.EntrypointConfigs = mapping.NewMap[string, EntrypointRuntime]()
	}
	name = strings.TrimSpace(name)
	if entrypoint.Name == "" {
		entrypoint.Name = name
	}
	address = strings.TrimSpace(address)
	if entrypoint.Address == "" {
		entrypoint.Address = address
	} else {
		address = entrypoint.Address
	}
	s.Entrypoints.Set(name, address)
	s.EntrypointConfigs.Set(name, entrypoint)
	return s
}

func (s *CompiledSnapshot) AddService(service *ServiceRuntime) *CompiledSnapshot {
	if s == nil || service == nil {
		return s
	}
	if s.Services == nil {
		s.Services = mapping.NewMap[string, *ServiceRuntime]()
	}
	s.Services.Set(service.Name, service)
	return s
}

func (s *CompiledSnapshot) AddRoute(route *CompiledRoute) *CompiledSnapshot {
	if s == nil || route == nil {
		return s
	}
	if s.RoutesByEntrypoint == nil {
		s.RoutesByEntrypoint = mapping.NewMultiMap[string, *CompiledRoute]()
	}
	s.RoutesByEntrypoint.Put(route.Entrypoint, route)
	return s
}

func NewService(name string, strategy string, endpoints ...*EndpointRuntime) *ServiceRuntime {
	if strategy == "" {
		strategy = "round_robin"
	}
	service := &ServiceRuntime{
		Name:      strings.TrimSpace(name),
		Strategy:  strings.TrimSpace(strategy),
		Endpoints: collectionlist.NewListWithCapacity[*EndpointRuntime](len(endpoints), endpoints...),
	}
	service.BuildSlots()
	return service
}

func NewEndpoint(rawURL string, weight int, proxy http.Handler) (*EndpointRuntime, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("endpoint url %q must include scheme and host", rawURL)
	}
	if weight <= 0 {
		weight = 1
	}
	endpoint := &EndpointRuntime{
		URL:    parsedURL,
		Weight: weight,
		Proxy:  proxy,
	}
	endpoint.Healthy.Store(true)
	return endpoint, nil
}

func NewRoute(name string, entrypoint string, service *ServiceRuntime) *CompiledRoute {
	return &CompiledRoute{
		Name:        strings.TrimSpace(name),
		Entrypoint:  strings.TrimSpace(entrypoint),
		Service:     service,
		Headers:     mapping.NewMap[string, string](),
		Predicates:  bitset.New(),
		Middlewares: collectionlist.NewList[MiddlewareRuntime](),
	}
}

func (r *CompiledRoute) WithHost(host string) *CompiledRoute {
	if r == nil {
		return nil
	}
	r.Host = strings.ToLower(strings.TrimSpace(host))
	if r.Host != "" {
		if r.Predicates == nil {
			r.Predicates = bitset.New()
		}
		r.Predicates.Set(PredicateHost)
	}
	return r
}

func (r *CompiledRoute) WithPathPrefix(pathPrefix string) *CompiledRoute {
	if r == nil {
		return nil
	}
	r.PathPrefix = strings.TrimSpace(pathPrefix)
	if r.PathPrefix != "" {
		if r.Predicates == nil {
			r.Predicates = bitset.New()
		}
		r.Predicates.Set(PredicatePathPrefix)
	}
	return r
}

func (r *CompiledRoute) WithMethod(method string) *CompiledRoute {
	if r == nil {
		return nil
	}
	r.Method = strings.ToUpper(strings.TrimSpace(method))
	if r.Method != "" {
		if r.Predicates == nil {
			r.Predicates = bitset.New()
		}
		r.Predicates.Set(PredicateMethod)
	}
	return r
}

func (r *CompiledRoute) WithHeader(key string, value string) *CompiledRoute {
	if r == nil {
		return nil
	}
	if r.Headers == nil {
		r.Headers = mapping.NewMap[string, string]()
	}
	r.Headers.Set(strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value))
	if !r.Headers.IsEmpty() {
		if r.Predicates == nil {
			r.Predicates = bitset.New()
		}
		r.Predicates.Set(PredicateHeaders)
	}
	return r
}

func (r *CompiledRoute) WithMiddleware(middleware MiddlewareRuntime) *CompiledRoute {
	if r == nil {
		return nil
	}
	if r.Middlewares == nil {
		r.Middlewares = collectionlist.NewList[MiddlewareRuntime]()
	}
	r.Middlewares.Add(middleware)
	return r
}

func NewMiddleware(name string) MiddlewareRuntime {
	return MiddlewareRuntime{
		Name:            strings.TrimSpace(name),
		Type:            MiddlewareTypeBuiltin,
		RequestHeaders:  mapping.NewMap[string, string](),
		ResponseHeaders: mapping.NewMap[string, string](),
	}
}

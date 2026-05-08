package runtime

import (
	"net/http"

	"github.com/arcgolabs/collectionx/mapping"
)

type routeHandlerIndex struct {
	snapshot *CompiledSnapshot
	handlers *mapping.Table[*CompiledRoute, *EndpointRuntime, http.Handler]
}

func newRouteHandlerIndex(snapshot *CompiledSnapshot, registry *MiddlewareRegistry) *routeHandlerIndex {
	index := &routeHandlerIndex{
		snapshot: snapshot,
		handlers: mapping.NewTable[*CompiledRoute, *EndpointRuntime, http.Handler](),
	}
	if snapshot == nil || snapshot.RoutesByEntrypoint == nil {
		return index
	}
	snapshot.RoutesByEntrypoint.Range(func(_ string, routes []*CompiledRoute) bool {
		for _, route := range routes {
			index.addRoute(route, registry)
		}
		return true
	})
	return index
}

func (i *routeHandlerIndex) addRoute(route *CompiledRoute, registry *MiddlewareRegistry) {
	if route == nil || route.Service == nil || route.Service.Endpoints == nil {
		return
	}
	route.Service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
		if endpoint == nil || endpoint.Proxy == nil {
			return true
		}
		i.handlers.Put(route, endpoint, WrapMiddlewaresWithRegistry(endpoint.Proxy, route.Middlewares, registry))
		return true
	})
}

func (i *routeHandlerIndex) handler(snapshot *CompiledSnapshot, route *CompiledRoute, endpoint *EndpointRuntime) (http.Handler, bool) {
	if i == nil || i.snapshot != snapshot || i.handlers == nil || route == nil || endpoint == nil {
		return nil, false
	}
	return i.handlers.Get(route, endpoint)
}

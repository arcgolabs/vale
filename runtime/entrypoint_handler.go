package runtime

import (
	"net/http"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type entrypointHandlerIndex struct {
	snapshot *CompiledSnapshot
	handlers *mapping.Map[string, *entrypointHandler]
}

type entrypointHandler struct {
	route    *CompiledRoute
	endpoint *EndpointRuntime
	handler  http.Handler
	matchAll bool
}

func newEntrypointHandlerIndex(snapshot *CompiledSnapshot, routes *routeHandlerIndex) *entrypointHandlerIndex {
	index := &entrypointHandlerIndex{
		snapshot: snapshot,
		handlers: mapping.NewMap[string, *entrypointHandler](),
	}
	if snapshot == nil || snapshot.RoutesByEntrypoint == nil {
		return index
	}
	snapshot.RoutesByEntrypoint.Range(func(entrypoint string, compiledRoutes []*CompiledRoute) bool {
		route, ok := singleCompiledRoute(compiledRoutes)
		if !ok || route == nil || route.Service == nil {
			return true
		}
		handler := &entrypointHandler{
			route:    route,
			matchAll: routeMatchesAllHTTP(route),
		}
		if endpoint, ok := singleServiceEndpoint(route.Service); ok {
			handler.endpoint = endpoint
			handler.handler, _ = routes.handler(snapshot, route, endpoint)
		}
		index.handlers.Set(entrypoint, handler)
		return true
	})
	return index
}

func (i *entrypointHandlerIndex) handler(snapshot *CompiledSnapshot, entrypoint string) (*entrypointHandler, bool) {
	if i == nil || i.snapshot != snapshot || i.handlers == nil {
		return nil, false
	}
	return i.handlers.Get(entrypoint)
}

func (h *entrypointHandler) matches(request *http.Request) bool {
	if h.matchAll {
		return true
	}
	return routeMatchesHTTP(h.route, request)
}

func singleCompiledRoute(routes []*CompiledRoute) (*CompiledRoute, bool) {
	if len(routes) != 1 {
		return nil, false
	}
	return collectionlist.NewList(routes...).GetFirst()
}

func singleServiceEndpoint(service *ServiceRuntime) (*EndpointRuntime, bool) {
	if service == nil || service.Endpoints == nil || service.Endpoints.Len() != 1 {
		return nil, false
	}
	return service.Endpoints.GetFirst()
}

func routeMatchesHTTP(route *CompiledRoute, request *http.Request) bool {
	if route == nil || request == nil {
		return false
	}
	return routeHostMatches(route, request) &&
		routePathMatches(route, request) &&
		routeMethodMatches(route, request) &&
		routeHeaderMatches(route, request)
}

func routeMatchesAllHTTP(route *CompiledRoute) bool {
	if route == nil {
		return false
	}
	return !hasPredicate(route, PredicateHost) &&
		!hasPredicate(route, PredicateMethod) &&
		!hasPredicate(route, PredicateHeaders) &&
		(!hasPredicate(route, PredicatePathPrefix) || route.PathPrefix == "/")
}

func routeHostMatches(route *CompiledRoute, request *http.Request) bool {
	return !hasPredicate(route, PredicateHost) || route.Host == normalizeRequestHost(request.Host)
}

func routePathMatches(route *CompiledRoute, request *http.Request) bool {
	return !hasPredicate(route, PredicatePathPrefix) || strings.HasPrefix(request.URL.Path, route.PathPrefix)
}

func routeMethodMatches(route *CompiledRoute, request *http.Request) bool {
	return !hasPredicate(route, PredicateMethod) || route.Method == normalizeRequestMethod(request.Method)
}

func routeHeaderMatches(route *CompiledRoute, request *http.Request) bool {
	return !hasPredicate(route, PredicateHeaders) || matchHeaders(route.Headers, request.Header)
}

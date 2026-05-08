package runtime

import (
	"net/http"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	lru "github.com/hashicorp/golang-lru/v2"
)

const (
	defaultRouteMatchCacheSize      = 4096
	defaultRouteMatchCacheMinRoutes = 16
)

type routeMatchCache struct {
	snapshot *CompiledSnapshot
	caches   *mapping.Map[string, *lru.Cache[routeMatchCacheKey, routeMatchCacheEntry]]
}

type routeMatchCacheKey struct {
	host   string
	path   string
	method string
}

type routeMatchCacheEntry struct {
	route *CompiledRoute
}

func newRouteMatchCache(snapshot *CompiledSnapshot) *routeMatchCache {
	cache := &routeMatchCache{
		snapshot: snapshot,
		caches:   mapping.NewMap[string, *lru.Cache[routeMatchCacheKey, routeMatchCacheEntry]](),
	}
	if snapshot == nil || snapshot.RoutesByEntrypoint == nil {
		return cache
	}
	snapshot.RoutesByEntrypoint.Range(func(entrypoint string, routes []*CompiledRoute) bool {
		if !routesAreCacheable(routes) {
			return true
		}
		entrypointCache, err := lru.New[routeMatchCacheKey, routeMatchCacheEntry](defaultRouteMatchCacheSize)
		if err == nil {
			cache.caches.Set(entrypoint, entrypointCache)
		}
		return true
	})
	return cache
}

func (c *routeMatchCache) get(
	snapshot *CompiledSnapshot,
	entrypoint string,
	request *http.Request,
) (*CompiledRoute, bool) {
	entrypointCache, ok := c.entrypointCache(snapshot, entrypoint)
	if !ok || request == nil {
		return nil, false
	}
	value, ok := entrypointCache.Get(newRouteMatchCacheKey(request))
	if !ok {
		return nil, false
	}
	return value.route, true
}

func (c *routeMatchCache) add(
	snapshot *CompiledSnapshot,
	entrypoint string,
	request *http.Request,
	route *CompiledRoute,
) {
	entrypointCache, ok := c.entrypointCache(snapshot, entrypoint)
	if !ok || request == nil {
		return
	}
	entrypointCache.Add(newRouteMatchCacheKey(request), routeMatchCacheEntry{route: route})
}

func (c *routeMatchCache) entrypointCache(
	snapshot *CompiledSnapshot,
	entrypoint string,
) (*lru.Cache[routeMatchCacheKey, routeMatchCacheEntry], bool) {
	if c == nil || c.snapshot != snapshot || c.caches == nil {
		return nil, false
	}
	return c.caches.Get(entrypoint)
}

func routesAreCacheable(routes []*CompiledRoute) bool {
	if len(routes) < defaultRouteMatchCacheMinRoutes {
		return false
	}
	return !collectionlist.NewList(routes...).AnyMatch(func(_ int, route *CompiledRoute) bool {
		return routeDependsOnHeaders(route)
	})
}

func routeDependsOnHeaders(route *CompiledRoute) bool {
	if route == nil {
		return false
	}
	if route.Predicates != nil {
		return route.Predicates.Contains(PredicateHeaders)
	}
	return route.Headers != nil && !route.Headers.IsEmpty()
}

func newRouteMatchCacheKey(request *http.Request) routeMatchCacheKey {
	return routeMatchCacheKey{
		host:   normalizeRequestHost(request.Host),
		path:   request.URL.Path,
		method: normalizeRequestMethod(request.Method),
	}
}

package runtime

import (
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/arcgolabs/collectionx/mapping"
)

type EntrypointMatcher struct {
	exactHosts *mapping.MultiMap[string, *CompiledRoute]
	wildcards  []wildcardBucket
	fallback   []*CompiledRoute
}

type wildcardBucket struct {
	suffix string
	routes []*CompiledRoute
}

func BuildEntrypointMatcher(routes []*CompiledRoute) *EntrypointMatcher {
	matcher := &EntrypointMatcher{
		exactHosts: mapping.NewMultiMap[string, *CompiledRoute](),
		wildcards:  make([]wildcardBucket, 0),
		fallback:   make([]*CompiledRoute, 0),
	}

	wildcardMap := mapping.NewMultiMap[string, *CompiledRoute]()
	for _, route := range routes {
		host := strings.TrimSpace(strings.ToLower(route.Host))
		switch {
		case host == "":
			matcher.fallback = append(matcher.fallback, route)
		case strings.HasPrefix(host, "*."):
			suffix := strings.TrimPrefix(host, "*")
			wildcardMap.Put(suffix, route)
		default:
			matcher.exactHosts.Put(host, route)
		}
	}

	matcher.exactHosts.Range(func(host string, hostRoutes []*CompiledRoute) bool {
		sortRoutesByPriority(hostRoutes)
		matcher.exactHosts.Set(host, hostRoutes...)
		return true
	})
	wildcardMap.Range(func(suffix string, wildcardRoutes []*CompiledRoute) bool {
		sortRoutesByPriority(wildcardRoutes)
		matcher.wildcards = append(matcher.wildcards, wildcardBucket{
			suffix: suffix,
			routes: wildcardRoutes,
		})
		return true
	})
	sort.Slice(matcher.wildcards, func(i, j int) bool {
		return len(matcher.wildcards[i].suffix) > len(matcher.wildcards[j].suffix)
	})
	sortRoutesByPriority(matcher.fallback)
	return matcher
}

func MatchRoute(matcher *EntrypointMatcher, routes []*CompiledRoute, request *http.Request) *CompiledRoute {
	// Backward-compatible path for old snapshots.
	if matcher == nil {
		return linearMatch(routes, request)
	}

	host := normalizeRequestHost(request.Host)
	method := strings.ToUpper(request.Method)

	if exactRoutes := matcher.exactHosts.Get(host); len(exactRoutes) > 0 {
		if route := matchWithPredicates(exactRoutes, request.URL.Path, method, request.Header); route != nil {
			return route
		}
	}

	for _, wildcard := range matcher.wildcards {
		if strings.HasSuffix(host, wildcard.suffix) {
			if route := matchWithPredicates(wildcard.routes, request.URL.Path, method, request.Header); route != nil {
				return route
			}
		}
	}

	return matchWithPredicates(matcher.fallback, request.URL.Path, method, request.Header)
}

func linearMatch(routes []*CompiledRoute, request *http.Request) *CompiledRoute {
	host := strings.ToLower(request.Host)
	method := strings.ToUpper(request.Method)
	for _, route := range routes {
		if route.Host != "" && route.Host != host {
			continue
		}
		if route.PathPrefix != "" && !strings.HasPrefix(request.URL.Path, route.PathPrefix) {
			continue
		}
		if route.Method != "" && route.Method != method {
			continue
		}
		if len(route.Headers) > 0 && !matchHeaders(route.Headers, request.Header) {
			continue
		}
		return route
	}
	return nil
}

func matchWithPredicates(routes []*CompiledRoute, path string, method string, headers http.Header) *CompiledRoute {
	for _, route := range routes {
		if route.PathPrefix != "" && !strings.HasPrefix(path, route.PathPrefix) {
			continue
		}
		if route.Method != "" && route.Method != method {
			continue
		}
		if len(route.Headers) > 0 && !matchHeaders(route.Headers, headers) {
			continue
		}
		return route
	}
	return nil
}

func sortRoutesByPriority(routes []*CompiledRoute) {
	sort.SliceStable(routes, func(i, j int) bool {
		left := routeScore(routes[i])
		right := routeScore(routes[j])
		if left == right {
			return routes[i].Name < routes[j].Name
		}
		return left > right
	})
}

func routeScore(route *CompiledRoute) int {
	score := 0
	score += len(route.PathPrefix) * 100
	if route.Method != "" {
		score += 10
	}
	score += len(route.Headers)
	return score
}

func normalizeRequestHost(hostPort string) string {
	hostPort = strings.ToLower(strings.TrimSpace(hostPort))
	if hostPort == "" {
		return hostPort
	}
	if host, _, err := net.SplitHostPort(hostPort); err == nil {
		return strings.ToLower(host)
	}
	return hostPort
}

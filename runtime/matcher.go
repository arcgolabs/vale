package runtime

import (
	"net/http"
	"slices"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/collectionx/prefix"
)

const (
	PredicateHost = iota
	PredicatePathPrefix
	PredicateMethod
	PredicateHeaders
)

type EntrypointMatcher struct {
	exactHosts *mapping.Map[string, *routeBucket]
	wildcards  *collectionlist.List[wildcardBucket]
	fallback   *routeBucket
}

type wildcardBucket struct {
	suffix string
	bucket *routeBucket
}

type routeBucket struct {
	pathRoutes *prefix.Trie[*collectionlist.List[*CompiledRoute]]
	fallback   *collectionlist.List[*CompiledRoute]
}

func BuildEntrypointMatcher(routes *collectionlist.List[*CompiledRoute]) *EntrypointMatcher {
	matcher := &EntrypointMatcher{
		exactHosts: mapping.NewMap[string, *routeBucket](),
		wildcards:  collectionlist.NewList[wildcardBucket](),
		fallback:   newRouteBucket(),
	}

	wildcardMap := mapping.NewMap[string, *routeBucket]()
	routes.Range(func(_ int, route *CompiledRoute) bool {
		host := strings.TrimSpace(strings.ToLower(route.Host))
		switch {
		case host == "":
			matcher.fallback.Add(route)
		case strings.HasPrefix(host, "*."):
			suffix := strings.TrimPrefix(host, "*")
			bucket, _ := wildcardMap.GetOrCompute(suffix, newRouteBucket)
			bucket.Add(route)
		default:
			bucket, _ := matcher.exactHosts.GetOrCompute(host, newRouteBucket)
			bucket.Add(route)
		}
		return true
	})

	matcher.exactHosts.Range(func(_ string, bucket *routeBucket) bool {
		bucket.Sort()
		return true
	})
	wildcardMap.Range(func(suffix string, bucket *routeBucket) bool {
		bucket.Sort()
		matcher.wildcards.Add(wildcardBucket{
			suffix: suffix,
			bucket: bucket,
		})
		return true
	})
	matcher.wildcards.Sort(func(left, right wildcardBucket) int {
		return len(right.suffix) - len(left.suffix)
	})
	matcher.fallback.Sort()
	return matcher
}

func MatchRoute(matcher *EntrypointMatcher, routes *collectionlist.List[*CompiledRoute], request *http.Request) *CompiledRoute {
	// Backward-compatible path for old snapshots.
	if matcher == nil {
		return linearMatch(routes, request)
	}

	host := normalizeRequestHost(request.Host)
	method := normalizeRequestMethod(request.Method)

	if exactBucket, ok := matcher.exactHosts.Get(host); ok {
		if route := exactBucket.Match(request.URL.Path, method, request.Header); route != nil {
			return route
		}
	}

	var wildcardRoute *CompiledRoute
	matcher.wildcards.Range(func(_ int, wildcard wildcardBucket) bool {
		if strings.HasSuffix(host, wildcard.suffix) {
			if route := wildcard.bucket.Match(request.URL.Path, method, request.Header); route != nil {
				wildcardRoute = route
				return false
			}
		}
		return true
	})
	if wildcardRoute != nil {
		return wildcardRoute
	}

	return matcher.fallback.Match(request.URL.Path, method, request.Header)
}

func matchSnapshotRoute(snapshot *CompiledSnapshot, entrypoint string, request *http.Request) *CompiledRoute {
	if snapshot == nil {
		return nil
	}
	if snapshot.EntrypointMatchers != nil {
		matcher, _ := snapshot.EntrypointMatchers.Get(entrypoint)
		if matcher != nil {
			return MatchRoute(matcher, nil, request)
		}
	}
	if snapshot.RoutesByEntrypoint == nil {
		return nil
	}
	return MatchRoute(nil, collectionlist.NewList(snapshot.RoutesByEntrypoint.Get(entrypoint)...), request)
}

func linearMatch(routes *collectionlist.List[*CompiledRoute], request *http.Request) *CompiledRoute {
	host := strings.ToLower(request.Host)
	method := normalizeRequestMethod(request.Method)
	matched, ok := collectionlist.FindList(routes, func(_ int, route *CompiledRoute) bool {
		return linearRouteMatches(route, request, host, method)
	})
	if !ok {
		return nil
	}
	return matched
}

func linearRouteMatches(route *CompiledRoute, request *http.Request, host, method string) bool {
	if hasPredicate(route, PredicateHost) && route.Host != host {
		return false
	}
	if hasPredicate(route, PredicatePathPrefix) && !strings.HasPrefix(request.URL.Path, route.PathPrefix) {
		return false
	}
	if hasPredicate(route, PredicateMethod) && route.Method != method {
		return false
	}
	if hasPredicate(route, PredicateHeaders) && !matchHeaders(route.Headers, request.Header) {
		return false
	}
	return true
}

func matchHeaders(expected *mapping.Map[string, string], actual http.Header) bool {
	if expected == nil {
		return true
	}
	matched := true
	expected.Range(func(key string, expectedValue string) bool {
		if !slices.Contains(actual.Values(key), expectedValue) {
			matched = false
			return false
		}
		return true
	})
	return matched
}

func matchWithPredicates(routes *collectionlist.List[*CompiledRoute], path, method string, headers http.Header) *CompiledRoute {
	matched, ok := collectionlist.FindList(routes, func(_ int, route *CompiledRoute) bool {
		return routeMatchesRequest(route, path, method, headers)
	})
	if !ok {
		return nil
	}
	return matched
}

func routeMatchesRequest(route *CompiledRoute, path, method string, headers http.Header) bool {
	if hasPredicate(route, PredicatePathPrefix) && !strings.HasPrefix(path, route.PathPrefix) {
		return false
	}
	if hasPredicate(route, PredicateMethod) && route.Method != method {
		return false
	}
	if hasPredicate(route, PredicateHeaders) && !matchHeaders(route.Headers, headers) {
		return false
	}
	return true
}

func sortRoutesByPriority(routes *collectionlist.List[*CompiledRoute]) *collectionlist.List[*CompiledRoute] {
	if routes == nil || routes.Len() < 2 {
		return routes
	}
	return routes.Clone().Sort(routePriorityCompare)
}

func routePriorityCompare(left, right *CompiledRoute) int {
	leftScore := routeScore(left)
	rightScore := routeScore(right)
	if leftScore == rightScore {
		return strings.Compare(left.Name, right.Name)
	}
	return rightScore - leftScore
}

func routeScore(route *CompiledRoute) int {
	score := 0
	score += len(route.PathPrefix) * 100
	if hasPredicate(route, PredicateMethod) {
		score += 10
	}
	if hasPredicate(route, PredicateHeaders) && route.Headers != nil {
		score += route.Headers.Len()
	}
	return score
}

func newRouteBucket() *routeBucket {
	return &routeBucket{
		pathRoutes: prefix.NewTrie[*collectionlist.List[*CompiledRoute]](),
		fallback:   collectionlist.NewList[*CompiledRoute](),
	}
}

func (b *routeBucket) Add(route *CompiledRoute) {
	if b == nil || route == nil {
		return
	}
	if !hasPredicate(route, PredicatePathPrefix) {
		b.fallback.Add(route)
		return
	}
	routes, _ := b.pathRoutes.Get(route.PathPrefix)
	if routes == nil {
		routes = collectionlist.NewList[*CompiledRoute]()
	}
	routes.Add(route)
	b.pathRoutes.Put(route.PathPrefix, routes)
}

func (b *routeBucket) Sort() {
	if b == nil {
		return
	}
	b.pathRoutes.RangePrefix("", func(pathPrefix string, routes *collectionlist.List[*CompiledRoute]) bool {
		b.pathRoutes.Put(pathPrefix, sortRoutesByPriority(routes))
		return true
	})
	b.fallback = sortRoutesByPriority(b.fallback)
}

func (b *routeBucket) Match(path, method string, headers http.Header) *CompiledRoute {
	if b == nil {
		return nil
	}
	for probe := path; probe != ""; {
		pathPrefix, routes, ok := b.pathRoutes.LongestPrefix(probe)
		if !ok {
			break
		}
		if route := matchWithPredicates(routes, path, method, headers); route != nil {
			return route
		}
		probe = trimLastRune(pathPrefix)
	}
	return matchWithPredicates(b.fallback, path, method, headers)
}

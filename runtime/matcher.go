package runtime

import (
	"net"
	"net/http"
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

func BuildEntrypointMatcher(routes []*CompiledRoute) *EntrypointMatcher {
	matcher := &EntrypointMatcher{
		exactHosts: mapping.NewMap[string, *routeBucket](),
		wildcards:  collectionlist.NewList[wildcardBucket](),
		fallback:   newRouteBucket(),
	}

	wildcardMap := mapping.NewMap[string, *routeBucket]()
	for _, route := range routes {
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
	}

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

func MatchRoute(matcher *EntrypointMatcher, routes []*CompiledRoute, request *http.Request) *CompiledRoute {
	// Backward-compatible path for old snapshots.
	if matcher == nil {
		return linearMatch(routes, request)
	}

	host := normalizeRequestHost(request.Host)
	method := strings.ToUpper(request.Method)

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

func linearMatch(routes []*CompiledRoute, request *http.Request) *CompiledRoute {
	host := strings.ToLower(request.Host)
	method := strings.ToUpper(request.Method)
	for _, route := range routes {
		if hasPredicate(route, PredicateHost) && route.Host != host {
			continue
		}
		if hasPredicate(route, PredicatePathPrefix) && !strings.HasPrefix(request.URL.Path, route.PathPrefix) {
			continue
		}
		if hasPredicate(route, PredicateMethod) && route.Method != method {
			continue
		}
		if hasPredicate(route, PredicateHeaders) && !matchHeaders(route.Headers, request.Header) {
			continue
		}
		return route
	}
	return nil
}

func matchWithPredicates(routes *collectionlist.List[*CompiledRoute], path string, method string, headers http.Header) *CompiledRoute {
	var matched *CompiledRoute
	routes.Range(func(_ int, route *CompiledRoute) bool {
		if hasPredicate(route, PredicatePathPrefix) && !strings.HasPrefix(path, route.PathPrefix) {
			return true
		}
		if hasPredicate(route, PredicateMethod) && route.Method != method {
			return true
		}
		if hasPredicate(route, PredicateHeaders) && !matchHeaders(route.Headers, headers) {
			return true
		}
		matched = route
		return false
	})
	return matched
}

func sortRoutesByPriority(routes *collectionlist.List[*CompiledRoute]) *collectionlist.List[*CompiledRoute] {
	if routes == nil || routes.Len() < 2 {
		return routes
	}
	queue, err := collectionlist.NewPriorityQueue(routePriorityLess, routes.Values()...)
	if err != nil {
		return routes
	}
	return collectionlist.NewList(queue.ValuesSorted()...)
}

func routePriorityLess(left *CompiledRoute, right *CompiledRoute) bool {
	leftScore := routeScore(left)
	rightScore := routeScore(right)
	if leftScore == rightScore {
		return left.Name < right.Name
	}
	return leftScore > rightScore
}

func routeScore(route *CompiledRoute) int {
	score := 0
	score += len(route.PathPrefix) * 100
	if hasPredicate(route, PredicateMethod) {
		score += 10
	}
	if hasPredicate(route, PredicateHeaders) {
		score += route.Headers.Len()
	}
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

func (b *routeBucket) Match(path string, method string, headers http.Header) *CompiledRoute {
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

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func hasPredicate(route *CompiledRoute, predicate int) bool {
	if route == nil {
		return false
	}
	if route.Predicates != nil {
		return route.Predicates.Contains(predicate)
	}
	switch predicate {
	case PredicateHost:
		return route.Host != ""
	case PredicatePathPrefix:
		return route.PathPrefix != ""
	case PredicateMethod:
		return route.Method != ""
	case PredicateHeaders:
		return route.Headers.Len() > 0
	default:
		return false
	}
}

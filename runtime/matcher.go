package runtime

import (
	"net"
	"net/http"
	"sort"
	"strings"

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
	wildcards  []wildcardBucket
	fallback   *routeBucket
}

type wildcardBucket struct {
	suffix string
	bucket *routeBucket
}

type routeBucket struct {
	pathRoutes   *prefix.Trie[[]*CompiledRoute]
	pathPrefixes []string
	fallback     []*CompiledRoute
}

func BuildEntrypointMatcher(routes []*CompiledRoute) *EntrypointMatcher {
	matcher := &EntrypointMatcher{
		exactHosts: mapping.NewMap[string, *routeBucket](),
		wildcards:  make([]wildcardBucket, 0),
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
		matcher.wildcards = append(matcher.wildcards, wildcardBucket{
			suffix: suffix,
			bucket: bucket,
		})
		return true
	})
	sort.Slice(matcher.wildcards, func(i, j int) bool {
		return len(matcher.wildcards[i].suffix) > len(matcher.wildcards[j].suffix)
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

	for _, wildcard := range matcher.wildcards {
		if strings.HasSuffix(host, wildcard.suffix) {
			if route := wildcard.bucket.Match(request.URL.Path, method, request.Header); route != nil {
				return route
			}
		}
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

func matchWithPredicates(routes []*CompiledRoute, path string, method string, headers http.Header) *CompiledRoute {
	for _, route := range routes {
		if hasPredicate(route, PredicatePathPrefix) && !strings.HasPrefix(path, route.PathPrefix) {
			continue
		}
		if hasPredicate(route, PredicateMethod) && route.Method != method {
			continue
		}
		if hasPredicate(route, PredicateHeaders) && !matchHeaders(route.Headers, headers) {
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
	if hasPredicate(route, PredicateMethod) {
		score += 10
	}
	if hasPredicate(route, PredicateHeaders) {
		score += len(route.Headers)
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
		pathRoutes: prefix.NewTrie[[]*CompiledRoute](),
	}
}

func (b *routeBucket) Add(route *CompiledRoute) {
	if b == nil || route == nil {
		return
	}
	if !hasPredicate(route, PredicatePathPrefix) {
		b.fallback = append(b.fallback, route)
		return
	}
	routes, _ := b.pathRoutes.Get(route.PathPrefix)
	if len(routes) == 0 {
		b.pathPrefixes = append(b.pathPrefixes, route.PathPrefix)
	}
	b.pathRoutes.Put(route.PathPrefix, append(routes, route))
}

func (b *routeBucket) Sort() {
	if b == nil {
		return
	}
	sort.Slice(b.pathPrefixes, func(i, j int) bool {
		return len(b.pathPrefixes[i]) > len(b.pathPrefixes[j])
	})
	for _, pathPrefix := range b.pathPrefixes {
		routes, _ := b.pathRoutes.Get(pathPrefix)
		sortRoutesByPriority(routes)
		b.pathRoutes.Put(pathPrefix, routes)
	}
	sortRoutesByPriority(b.fallback)
}

func (b *routeBucket) Match(path string, method string, headers http.Header) *CompiledRoute {
	if b == nil {
		return nil
	}
	for _, pathPrefix := range b.pathPrefixes {
		if !strings.HasPrefix(path, pathPrefix) {
			continue
		}
		routes, _ := b.pathRoutes.Get(pathPrefix)
		if route := matchWithPredicates(routes, path, method, headers); route != nil {
			return route
		}
	}
	return matchWithPredicates(b.fallback, path, method, headers)
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
		return len(route.Headers) > 0
	default:
		return false
	}
}

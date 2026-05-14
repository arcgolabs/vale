package runtime

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
)

const MiddlewareTypeBuiltin = "builtin"

type MiddlewareFactory func(http.Handler, MiddlewareRuntime) http.Handler

type MiddlewareRegistry struct {
	factories *mapping.Map[string, MiddlewareFactory]
}

func NewMiddlewareRegistry() *MiddlewareRegistry {
	return &MiddlewareRegistry{factories: mapping.NewMap[string, MiddlewareFactory]()}
}

func DefaultMiddlewareRegistry() *MiddlewareRegistry {
	registry := NewMiddlewareRegistry()
	registry.factories.Set(MiddlewareTypeBuiltin, wrapBuiltinMiddleware)
	return registry
}

func (r *MiddlewareRegistry) Register(middlewareType string, factory MiddlewareFactory) error {
	if r == nil {
		return oops.
			In("runtime").
			New("middleware registry cannot be nil")
	}
	if factory == nil {
		return oops.
			In("runtime").
			With("middleware_type", middlewareType).
			New("middleware factory cannot be nil")
	}
	if r.factories == nil {
		r.factories = mapping.NewMap[string, MiddlewareFactory]()
	}
	middlewareType = normalizeMiddlewareType(middlewareType)
	r.factories.Set(middlewareType, factory)
	return nil
}

func (r *MiddlewareRegistry) Factory(middlewareType string) (MiddlewareFactory, bool) {
	if r == nil || r.factories == nil {
		return nil, false
	}
	return r.factories.Get(normalizeMiddlewareType(middlewareType))
}

func (r *MiddlewareRegistry) Names() *collectionlist.List[string] {
	if r == nil || r.factories == nil {
		return collectionlist.NewList[string]()
	}
	return collectionlist.NewList(r.factories.Keys()...).Sort(strings.Compare)
}

func (r *MiddlewareRegistry) Clone() *MiddlewareRegistry {
	if r == nil || r.factories == nil {
		return NewMiddlewareRegistry()
	}
	return &MiddlewareRegistry{factories: r.factories.Clone()}
}

func WrapMiddlewares(handler http.Handler, middlewares *collectionlist.List[MiddlewareRuntime]) http.Handler {
	return WrapMiddlewaresWithRegistry(handler, middlewares, nil)
}

func WrapMiddlewaresWithRegistry(handler http.Handler, middlewares *collectionlist.List[MiddlewareRuntime], registry *MiddlewareRegistry) http.Handler {
	if registry == nil {
		registry = DefaultMiddlewareRegistry()
	}
	for index := middlewares.Len() - 1; index >= 0; index-- {
		middleware, _ := middlewares.Get(index)
		factory, ok := registry.Factory(middleware.Type)
		if !ok {
			factory, _ = registry.Factory(MiddlewareTypeBuiltin)
		}
		if factory == nil {
			continue
		}
		handler = factory(handler, middleware)
	}
	return handler
}

func wrapBuiltinMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	replacePathRegex := compileMiddlewareRegex(middleware.ReplacePathRegex)
	redirectRegex := compileMiddlewareRegex(middleware.RedirectRegex)
	handler := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware.MaxBodyBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, middleware.MaxBodyBytes)
		}
		applyHeaders(r.Header, middleware.RequestHeaders)
		applyPathMiddleware(r, middleware, replacePathRegex)
		if target, ok := redirectTarget(r, middleware, redirectRegex); ok {
			writeRedirect(w, target, redirectStatus(middleware.RedirectPermanent))
			return
		}
		applyHeaders(w.Header(), middleware.ResponseHeaders)
		next.ServeHTTP(w, r)
	}))
	handler = wrapCircuitBreakerMiddleware(handler, middleware)
	handler = wrapRateLimitMiddleware(handler, middleware)
	handler = wrapForwardAuthMiddleware(handler, middleware)
	handler = wrapIPAllowListMiddleware(handler, middleware)
	handler = wrapBasicAuthMiddleware(handler, middleware)
	handler = wrapCompressMiddleware(handler, middleware)
	handler = wrapCORSMiddleware(handler, middleware)
	handler = wrapSecureMiddleware(handler, middleware)
	return handler
}

func applyPathMiddleware(r *http.Request, middleware MiddlewareRuntime, replacePathRegex *regexp.Regexp) {
	stripPrefixes(r, middleware)
	if middleware.ReplacePath != "" {
		r.URL.Path = ensurePath(middleware.ReplacePath)
	}
	if replacePathRegex != nil {
		replacement := middleware.ReplacePathReplacement
		r.URL.Path = ensurePath(replacePathRegex.ReplaceAllString(r.URL.Path, replacement))
	}
	if middleware.AddPrefix != "" {
		r.URL.Path = joinPathPrefix(middleware.AddPrefix, r.URL.Path)
	}
}

func stripPrefixes(r *http.Request, middleware MiddlewareRuntime) {
	if middleware.StripPrefix != "" {
		stripPrefix(r, middleware.StripPrefix)
	}
	if middleware.StripPrefixes == nil {
		return
	}
	middleware.StripPrefixes.Range(func(_ int, prefix string) bool {
		stripPrefix(r, prefix)
		return true
	})
}

func stripPrefix(r *http.Request, prefix string) {
	if prefix == "" || !strings.HasPrefix(r.URL.Path, prefix) {
		return
	}
	r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
	r.URL.Path = ensurePath(r.URL.Path)
}

func applyHeaders(headers http.Header, values *mapping.Map[string, string]) {
	if values == nil {
		return
	}
	values.Range(func(key string, value string) bool {
		headers.Set(key, value)
		return true
	})
}

func redirectTarget(r *http.Request, middleware MiddlewareRuntime, redirectRegex *regexp.Regexp) (string, bool) {
	if middleware.RedirectScheme != "" {
		return schemeRedirectTarget(r, middleware), true
	}
	if redirectRegex == nil {
		return "", false
	}
	current := requestAbsoluteURL(r)
	if !redirectRegex.MatchString(current) {
		return "", false
	}
	return redirectRegex.ReplaceAllString(current, middleware.RedirectReplacement), true
}

func writeRedirect(w http.ResponseWriter, target string, status int) {
	location, ok := safeRedirectLocation(target)
	if !ok {
		http.Error(w, "invalid redirect target", http.StatusBadRequest)
		return
	}
	w.Header().Set("Location", location)
	w.WriteHeader(status)
}

func safeRedirectLocation(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.ContainsAny(target, "\r\n") {
		return "", false
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", false
	}
	if parsed.IsAbs() {
		if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return "", false
		}
		return parsed.String(), true
	}
	if strings.HasPrefix(target, "//") || !strings.HasPrefix(target, "/") {
		return "", false
	}
	return parsed.String(), true
}

func schemeRedirectTarget(r *http.Request, middleware MiddlewareRuntime) string {
	target := *r.URL
	target.Scheme = middleware.RedirectScheme
	target.Host = redirectHost(r.Host, middleware.RedirectPort)
	return target.String()
}

func redirectHost(host, port string) string {
	if strings.TrimSpace(port) == "" {
		return host
	}
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	return net.JoinHostPort(name, port)
}

func requestAbsoluteURL(r *http.Request) string {
	target := *r.URL
	if target.Scheme == "" {
		target.Scheme = requestScheme(r)
	}
	if target.Host == "" {
		target.Host = r.Host
	}
	return target.String()
}

func requestScheme(r *http.Request) string {
	if scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); scheme != "" {
		return scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

func redirectStatus(permanent bool) int {
	if permanent {
		return http.StatusMovedPermanently
	}
	return http.StatusFound
}

func compileMiddlewareRegex(pattern string) *regexp.Regexp {
	if strings.TrimSpace(pattern) == "" {
		return nil
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return compiled
}

func normalizeMiddlewareType(middlewareType string) string {
	middlewareType = strings.ToLower(strings.TrimSpace(middlewareType))
	if middlewareType == "" {
		return MiddlewareTypeBuiltin
	}
	return middlewareType
}

func joinPathPrefix(prefix, path string) string {
	prefix = "/" + strings.Trim(prefix, "/")
	path = "/" + strings.TrimLeft(path, "/")
	if prefix == "/" {
		return path
	}
	if path == "/" {
		return prefix
	}
	return prefix + path
}

func ensurePath(path string) string {
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

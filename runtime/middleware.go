package runtime

import (
	"net/http"
	"regexp"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/internal/genericx"
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

func (r *MiddlewareRegistry) Register[H http.Handler](
	middlewareType string,
	factory func(http.Handler, MiddlewareRuntime) H,
) error {
	if genericx.IsNil(factory) {
		return oops.
			In("runtime").
			With("middleware_type", middlewareType).
			New("middleware factory cannot be nil")
	}
	return r.RegisterFactory(middlewareType, func(next http.Handler, middleware MiddlewareRuntime) http.Handler {
		return factory(next, middleware)
	})
}

func (r *MiddlewareRegistry) RegisterFactory(middlewareType string, factory MiddlewareFactory) error {
	if r == nil {
		return oops.
			In("runtime").
			New("middleware registry cannot be nil")
	}
	if genericx.IsNil(factory) {
		return oops.
			In("runtime").
			With("middleware_type", middlewareType).
			New("middleware factory cannot be nil")
	}
	if r.factories == nil {
		r.factories = mapping.NewMap[string, MiddlewareFactory]()
	}
	middlewareType = normalizeMiddlewareType(middlewareType)
	r.factories.Set(middlewareType, func(next http.Handler, middleware MiddlewareRuntime) http.Handler {
		wrapped := factory(next, middleware)
		if genericx.IsNil(wrapped) {
			return next
		}
		return wrapped
	})
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

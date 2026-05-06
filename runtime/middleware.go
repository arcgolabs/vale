package runtime

import (
	"fmt"
	"net/http"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
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
	_ = registry.Register(MiddlewareTypeBuiltin, wrapBuiltinMiddleware)
	return registry
}

func (r *MiddlewareRegistry) Register(middlewareType string, factory MiddlewareFactory) error {
	if r == nil {
		return fmt.Errorf("middleware registry cannot be nil")
	}
	if factory == nil {
		return fmt.Errorf("middleware factory cannot be nil")
	}
	if r.factories == nil {
		r.factories = mapping.NewMap[string, MiddlewareFactory]()
	}
	middlewareType = normalizeMiddlewareType(middlewareType)
	r.factories.Set(middlewareType, factory)
	return nil
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
		factory, ok := registry.factories.Get(normalizeMiddlewareType(middleware.Type))
		if !ok {
			factory, _ = registry.factories.Get(MiddlewareTypeBuiltin)
		}
		if factory == nil {
			continue
		}
		handler = factory(handler, middleware)
	}
	return handler
}

func wrapBuiltinMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware.MaxBodyBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, middleware.MaxBodyBytes)
		}
		middleware.RequestHeaders.Range(func(key string, value string) bool {
			r.Header.Set(key, value)
			return true
		})
		if middleware.StripPrefix != "" && strings.HasPrefix(r.URL.Path, middleware.StripPrefix) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, middleware.StripPrefix)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}
		if middleware.AddPrefix != "" {
			r.URL.Path = joinPathPrefix(middleware.AddPrefix, r.URL.Path)
		}
		middleware.ResponseHeaders.Range(func(key string, value string) bool {
			w.Header().Set(key, value)
			return true
		})
		next.ServeHTTP(w, r)
	})
}

func normalizeMiddlewareType(middlewareType string) string {
	middlewareType = strings.ToLower(strings.TrimSpace(middlewareType))
	if middlewareType == "" {
		return MiddlewareTypeBuiltin
	}
	return middlewareType
}

func joinPathPrefix(prefix string, path string) string {
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

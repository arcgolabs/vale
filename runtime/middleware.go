package runtime

import (
	"net/http"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func WrapMiddlewares(handler http.Handler, middlewares *collectionlist.List[MiddlewareRuntime]) http.Handler {
	for index := middlewares.Len() - 1; index >= 0; index-- {
		middleware, _ := middlewares.Get(index)
		handler = wrapMiddleware(handler, middleware)
	}
	return handler
}

func wrapMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
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

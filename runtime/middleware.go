package runtime

import (
	"net/http"
	"strings"
)

func WrapMiddlewares(handler http.Handler, middlewares []MiddlewareRuntime) http.Handler {
	for index := len(middlewares) - 1; index >= 0; index-- {
		handler = wrapMiddleware(handler, middlewares[index])
	}
	return handler
}

func wrapMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware.MaxBodyBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, middleware.MaxBodyBytes)
		}
		for key, value := range middleware.RequestHeaders {
			r.Header.Set(key, value)
		}
		if middleware.StripPrefix != "" && strings.HasPrefix(r.URL.Path, middleware.StripPrefix) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, middleware.StripPrefix)
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}
		if middleware.AddPrefix != "" {
			r.URL.Path = joinPathPrefix(middleware.AddPrefix, r.URL.Path)
		}
		for key, value := range middleware.ResponseHeaders {
			w.Header().Set(key, value)
		}
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

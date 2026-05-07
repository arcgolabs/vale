package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/config"
)

func (b *ConfigBuilder) MiddlewareNamed(name string, options ...MiddlewareOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	middleware := config.Middleware{
		Name:            strings.TrimSpace(name),
		RequestHeaders:  map[string]string{},
		ResponseHeaders: map[string]string{},
	}
	if middleware.Name == "" {
		b.addError("middleware name cannot be empty")
	}
	collectionlist.NewList(options...).Range(func(_ int, option MiddlewareOption) bool {
		if option != nil {
			option(&middleware)
		}
		return true
	})
	b.middlewares.Add(middleware)
	return b
}

func (b *ConfigBuilder) Middleware(middleware config.Middleware) *ConfigBuilder {
	if b == nil {
		return nil
	}
	if strings.TrimSpace(middleware.Name) == "" {
		b.addError("middleware name cannot be empty")
	}
	b.middlewares.Add(middleware)
	return b
}

func MiddlewareType(middlewareType string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.Type = strings.TrimSpace(middlewareType)
		}
	}
}

func MiddlewareStripPrefix(pathPrefix string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.StripPrefix = strings.TrimSpace(pathPrefix)
		}
	}
}

func MiddlewareStripPrefixes(pathPrefixes ...string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		middleware.StripPrefixes = cleanStrings(pathPrefixes)
	}
}

func MiddlewareAddPrefix(pathPrefix string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.AddPrefix = strings.TrimSpace(pathPrefix)
		}
	}
}

func MiddlewareReplacePath(path string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.ReplacePath = strings.TrimSpace(path)
		}
	}
}

func MiddlewareReplacePathRegex(pattern, replacement string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		middleware.ReplacePathRegex = strings.TrimSpace(pattern)
		middleware.ReplacePathReplacement = strings.TrimSpace(replacement)
	}
}

func MiddlewareRedirectScheme(scheme, port string, permanent bool) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		middleware.RedirectScheme = strings.TrimSpace(scheme)
		middleware.RedirectPort = strings.TrimSpace(port)
		middleware.RedirectPermanent = permanent
	}
}

func MiddlewareRedirectRegex(pattern, replacement string, permanent bool) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		middleware.RedirectRegex = strings.TrimSpace(pattern)
		middleware.RedirectReplacement = strings.TrimSpace(replacement)
		middleware.RedirectPermanent = permanent
	}
}

func MiddlewareChain(names ...string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.Chain = cleanStrings(names)
		}
	}
}

func MiddlewareRequestHeader(key, value string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		if middleware.RequestHeaders == nil {
			middleware.RequestHeaders = map[string]string{}
		}
		middleware.RequestHeaders[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func MiddlewareResponseHeader(key, value string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		if middleware.ResponseHeaders == nil {
			middleware.ResponseHeaders = map[string]string{}
		}
		middleware.ResponseHeaders[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
}

func MiddlewareMaxBodyBytes(maxBodyBytes int64) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.MaxBodyBytes = maxBodyBytes
		}
	}
}

func cleanStrings(values []string) []string {
	return collectionlist.FilterMapList(collectionlist.NewList(values...), func(_ int, value string) (string, bool) {
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed != ""
	}).Values()
}

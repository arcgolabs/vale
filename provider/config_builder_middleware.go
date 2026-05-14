package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/config"
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
		middleware.StripPrefixes = cleanStrings(collectionlist.NewList(pathPrefixes...)).Values()
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
			middleware.Chain = cleanStrings(collectionlist.NewList(names...)).Values()
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

func MiddlewareBasicAuth(realm string, users map[string]string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.BasicAuth = &config.BasicAuth{
				Enabled: true,
				Realm:   strings.TrimSpace(realm),
				Users:   users,
			}
		}
	}
}

func MiddlewareCompress(minBytes int) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.Compress = &config.Compress{
				Enabled:  true,
				MinBytes: minBytes,
			}
		}
	}
}

func MiddlewareIPAllowList(trustForwardHeader bool, sourceRange ...string) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware != nil {
			middleware.IPAllowList = &config.IPAllowList{
				Enabled:            true,
				SourceRange:        cleanStrings(collectionlist.NewList(sourceRange...)).Values(),
				TrustForwardHeader: trustForwardHeader,
			}
		}
	}
}

type ForwardAuthOption func(*config.ForwardAuth)

func MiddlewareForwardAuth(address string, options ...ForwardAuthOption) MiddlewareOption {
	return func(middleware *config.Middleware) {
		if middleware == nil {
			return
		}
		forwardAuth := &config.ForwardAuth{
			Enabled: true,
			Address: strings.TrimSpace(address),
		}
		collectionlist.NewList(options...).Range(func(_ int, option ForwardAuthOption) bool {
			if option != nil {
				option(forwardAuth)
			}
			return true
		})
		middleware.ForwardAuth = forwardAuth
	}
}

func ForwardAuthTimeout(timeout string) ForwardAuthOption {
	return func(forwardAuth *config.ForwardAuth) {
		if forwardAuth != nil {
			forwardAuth.Timeout = strings.TrimSpace(timeout)
		}
	}
}

func ForwardAuthTrustForwardHeader(enabled bool) ForwardAuthOption {
	return func(forwardAuth *config.ForwardAuth) {
		if forwardAuth != nil {
			forwardAuth.TrustForwardHeader = enabled
		}
	}
}

func ForwardAuthForwardBody(maxBodyBytes int64) ForwardAuthOption {
	return func(forwardAuth *config.ForwardAuth) {
		if forwardAuth != nil {
			forwardAuth.ForwardBody = true
			forwardAuth.MaxBodyBytes = maxBodyBytes
		}
	}
}

func ForwardAuthRequestHeaders(headers ...string) ForwardAuthOption {
	return func(forwardAuth *config.ForwardAuth) {
		if forwardAuth != nil {
			forwardAuth.AuthRequestHeaders = cleanStrings(collectionlist.NewList(headers...)).Values()
		}
	}
}

func ForwardAuthResponseHeaders(headers ...string) ForwardAuthOption {
	return func(forwardAuth *config.ForwardAuth) {
		if forwardAuth != nil {
			forwardAuth.AuthResponseHeaders = cleanStrings(collectionlist.NewList(headers...)).Values()
		}
	}
}

func ForwardAuthMaxResponseBodyBytes(maxBodyBytes int64) ForwardAuthOption {
	return func(forwardAuth *config.ForwardAuth) {
		if forwardAuth != nil {
			forwardAuth.MaxResponseBodyBytes = maxBodyBytes
		}
	}
}

func cleanStrings(values *collectionlist.List[string]) *collectionlist.List[string] {
	return collectionlist.FilterMapList(values, func(_ int, value string) (string, bool) {
		trimmed := strings.TrimSpace(value)
		return trimmed, trimmed != ""
	})
}

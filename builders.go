package vela

import (
	"net/http"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
	"github.com/samber/oops"
)

type (
	RuntimeSnapshot    = runtime.CompiledSnapshot
	RuntimeRoute       = runtime.CompiledRoute
	RuntimeService     = runtime.ServiceRuntime
	RuntimeEndpoint    = runtime.EndpointRuntime
	RuntimeEntrypoint  = runtime.EntrypointRuntime
	RuntimeTLS         = runtime.TLSRuntime
	RuntimeACME        = runtime.ACMERuntime
	RuntimeSecurity    = runtime.SecurityRuntime
	RuntimeMiddleware  = runtime.MiddlewareRuntime
	RuntimeCatalog     = runtime.Catalog
	RuntimeRouteFilter = runtime.RouteFilter
	RuntimeRouteRecord = runtime.RouteRecord
	MiddlewareRegistry = runtime.MiddlewareRegistry
	MiddlewareFactory  = runtime.MiddlewareFactory
	ConfigEndpoint     = config.Endpoint
	ConfigRoute        = config.Route
	ConfigMiddleware   = config.Middleware
	ConfigSecurity     = config.Security
	ConfigBuilder      = provider.ConfigBuilder
	EntrypointOption   = provider.EntrypointOption
	RouteOption        = provider.RouteOption
	MiddlewareOption   = provider.MiddlewareOption
)

func NewSnapshot() *RuntimeSnapshot {
	return runtime.NewSnapshot()
}

func NewService(name, strategy string, endpoints ...*RuntimeEndpoint) *RuntimeService {
	return runtime.NewService(name, strategy, endpoints...)
}

func NewEndpoint(rawURL string, weight int, proxy http.Handler) (*RuntimeEndpoint, error) {
	endpoint, err := runtime.NewEndpoint(rawURL, weight, proxy)
	if err != nil {
		return nil, oops.
			In("vela").
			With("url", rawURL, "weight", weight).
			Wrapf(err, "create runtime endpoint")
	}
	return endpoint, nil
}

func NewRoute(name, entrypoint string, service *RuntimeService) *RuntimeRoute {
	return runtime.NewRoute(name, entrypoint, service)
}

func NewMiddleware(name string) RuntimeMiddleware {
	return runtime.NewMiddleware(name)
}

func NewMiddlewareRegistry() *MiddlewareRegistry {
	return runtime.NewMiddlewareRegistry()
}

func DefaultMiddlewareRegistry() *MiddlewareRegistry {
	return runtime.DefaultMiddlewareRegistry()
}

func NewConfigBuilder() *ConfigBuilder {
	return provider.NewConfigBuilder()
}

func NewConfigEndpoint(rawURL string, weight int) ConfigEndpoint {
	return provider.ConfigEndpoint(rawURL, weight)
}

func EntrypointTLS(certFile, keyFile string) EntrypointOption {
	return provider.EntrypointTLS(certFile, keyFile)
}

func EntrypointACME(email, cacheDir string, domains ...string) EntrypointOption {
	return provider.EntrypointACME(email, cacheDir, domains...)
}

func RouteHost(host string) RouteOption {
	return provider.RouteHost(host)
}

func RoutePathPrefix(pathPrefix string) RouteOption {
	return provider.RoutePathPrefix(pathPrefix)
}

func RouteMethod(method string) RouteOption {
	return provider.RouteMethod(method)
}

func RouteHeader(key, value string) RouteOption {
	return provider.RouteHeader(key, value)
}

func RouteMiddlewares(names ...string) RouteOption {
	return provider.RouteMiddlewares(names...)
}

func MiddlewareType(middlewareType string) MiddlewareOption {
	return provider.MiddlewareType(middlewareType)
}

func MiddlewareStripPrefix(pathPrefix string) MiddlewareOption {
	return provider.MiddlewareStripPrefix(pathPrefix)
}

func MiddlewareStripPrefixes(pathPrefixes ...string) MiddlewareOption {
	return provider.MiddlewareStripPrefixes(pathPrefixes...)
}

func MiddlewareAddPrefix(pathPrefix string) MiddlewareOption {
	return provider.MiddlewareAddPrefix(pathPrefix)
}

func MiddlewareReplacePath(path string) MiddlewareOption {
	return provider.MiddlewareReplacePath(path)
}

func MiddlewareReplacePathRegex(pattern, replacement string) MiddlewareOption {
	return provider.MiddlewareReplacePathRegex(pattern, replacement)
}

func MiddlewareRedirectScheme(scheme, port string, permanent bool) MiddlewareOption {
	return provider.MiddlewareRedirectScheme(scheme, port, permanent)
}

func MiddlewareRedirectRegex(pattern, replacement string, permanent bool) MiddlewareOption {
	return provider.MiddlewareRedirectRegex(pattern, replacement, permanent)
}

func MiddlewareChain(names ...string) MiddlewareOption {
	return provider.MiddlewareChain(names...)
}

func MiddlewareRequestHeader(key, value string) MiddlewareOption {
	return provider.MiddlewareRequestHeader(key, value)
}

func MiddlewareResponseHeader(key, value string) MiddlewareOption {
	return provider.MiddlewareResponseHeader(key, value)
}

func MiddlewareMaxBodyBytes(maxBodyBytes int64) MiddlewareOption {
	return provider.MiddlewareMaxBodyBytes(maxBodyBytes)
}

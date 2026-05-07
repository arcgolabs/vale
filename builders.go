package vela

import (
	"net/http"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	"github.com/arcgolabs/vela/runtime"
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

func NewService(name string, strategy string, endpoints ...*RuntimeEndpoint) *RuntimeService {
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

func NewRoute(name string, entrypoint string, service *RuntimeService) *RuntimeRoute {
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

func EntrypointTLS(certFile string, keyFile string) EntrypointOption {
	return provider.EntrypointTLS(certFile, keyFile)
}

func EntrypointACME(email string, cacheDir string, domains ...string) EntrypointOption {
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

func RouteHeader(key string, value string) RouteOption {
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

func MiddlewareAddPrefix(pathPrefix string) MiddlewareOption {
	return provider.MiddlewareAddPrefix(pathPrefix)
}

func MiddlewareRequestHeader(key string, value string) MiddlewareOption {
	return provider.MiddlewareRequestHeader(key, value)
}

func MiddlewareResponseHeader(key string, value string) MiddlewareOption {
	return provider.MiddlewareResponseHeader(key, value)
}

func MiddlewareMaxBodyBytes(maxBodyBytes int64) MiddlewareOption {
	return provider.MiddlewareMaxBodyBytes(maxBodyBytes)
}

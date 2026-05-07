package vela

import (
	"net/http"

	"github.com/arcgolabs/vela/provider"
	"github.com/arcgolabs/vela/runtime"
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
	MiddlewareRegistry = runtime.MiddlewareRegistry
	MiddlewareFactory  = runtime.MiddlewareFactory
	ConfigBuilder      = provider.ConfigBuilder
)

func NewSnapshot() *RuntimeSnapshot {
	return runtime.NewSnapshot()
}

func NewService(name string, strategy string, endpoints ...*RuntimeEndpoint) *RuntimeService {
	return runtime.NewService(name, strategy, endpoints...)
}

func NewEndpoint(rawURL string, weight int, proxy http.Handler) (*RuntimeEndpoint, error) {
	return runtime.NewEndpoint(rawURL, weight, proxy)
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

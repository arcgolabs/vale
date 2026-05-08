package provider

import (
	"net/url"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/config"
)

func (b *ConfigBuilder) Service(name, endpointURL string) *ConfigBuilder {
	return b.ServiceWithEndpoints(name, ConfigEndpoint(endpointURL, 1))
}

func (b *ConfigBuilder) ServiceWithEndpoints(name string, endpoints ...config.Endpoint) *ConfigBuilder {
	return b.ServiceWithStrategy(name, "round_robin", endpoints...)
}

func (b *ConfigBuilder) ServiceWithStrategy(name, strategy string, endpoints ...config.Endpoint) *ConfigBuilder {
	if b == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	strategy = defaultServiceStrategy(strategy)
	b.validateService(name, strategy, len(endpoints))
	b.services.Add(config.Service{
		Name:      name,
		Strategy:  strategy,
		Endpoints: b.validatedEndpoints(name, endpoints).Values(),
	})
	return b
}

func defaultServiceStrategy(strategy string) string {
	strategy = strings.TrimSpace(strategy)
	if strategy == "" {
		return "round_robin"
	}
	return strategy
}

func (b *ConfigBuilder) validateService(name, strategy string, endpointCount int) {
	if name == "" {
		b.addError("service name cannot be empty")
	}
	if strategy != "round_robin" && strategy != "weighted_round_robin" {
		b.addError("service %q has unsupported strategy %q", name, strategy)
	}
	if endpointCount == 0 {
		b.addError("service %q must have at least one endpoint", name)
	}
}

func (b *ConfigBuilder) validatedEndpoints(name string, endpoints []config.Endpoint) *collectionlist.List[config.Endpoint] {
	endpointList := collectionlist.NewListWithCapacity[config.Endpoint](len(endpoints))
	for _, endpoint := range endpoints {
		endpointList.Add(b.validatedEndpoint(name, endpoint))
	}
	return endpointList
}

func (b *ConfigBuilder) validatedEndpoint(name string, endpoint config.Endpoint) config.Endpoint {
	endpoint.URL = strings.TrimSpace(endpoint.URL)
	if endpoint.URL == "" {
		b.addError("service %q endpoint url cannot be empty", name)
	} else if parsed, err := url.Parse(endpoint.URL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		b.addError("service %q endpoint %q is invalid", name, endpoint.URL)
	}
	if endpoint.Weight <= 0 {
		endpoint.Weight = 1
	}
	return endpoint
}

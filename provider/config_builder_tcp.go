package provider

import (
	"net"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/config"
)

func ConfigTCPEndpoint(address string, weight int) config.TCPEndpoint {
	if weight <= 0 {
		weight = 1
	}
	return config.TCPEndpoint{Address: strings.TrimSpace(address), Weight: weight}
}

func (b *ConfigBuilder) TCPService(name, endpointAddress string) *ConfigBuilder {
	return b.TCPServiceWithEndpoints(name, ConfigTCPEndpoint(endpointAddress, 1))
}

func (b *ConfigBuilder) TCPServiceWithEndpoints(name string, endpoints ...config.TCPEndpoint) *ConfigBuilder {
	return b.TCPServiceWithStrategy(name, "round_robin", endpoints...)
}

func (b *ConfigBuilder) TCPServiceWithStrategy(name, strategy string, endpoints ...config.TCPEndpoint) *ConfigBuilder {
	if b == nil {
		return nil
	}
	name = strings.TrimSpace(name)
	strategy = defaultServiceStrategy(strategy)
	b.validateTCPService(name, strategy, len(endpoints))
	b.tcpServices.Add(config.TCPService{
		Name:      name,
		Strategy:  strategy,
		Endpoints: b.validatedTCPEndpoints(name, endpoints).Values(),
	})
	return b
}

func (b *ConfigBuilder) TCPRouteTo(name, entrypoint, service string, options ...TCPRouteOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	route := config.TCPRoute{
		Name:       strings.TrimSpace(name),
		Entrypoint: strings.TrimSpace(entrypoint),
		Service:    strings.TrimSpace(service),
	}
	b.validateTCPRoute(route)
	collectionlist.NewList(options...).Range(func(_ int, option TCPRouteOption) bool {
		if option != nil {
			option(&route)
		}
		return true
	})
	b.tcpRoutes.Add(route)
	return b
}

func (b *ConfigBuilder) TCPRoute(route config.TCPRoute) *ConfigBuilder {
	if b == nil {
		return nil
	}
	if strings.TrimSpace(route.Name) == "" {
		b.addError("tcp_route name cannot be empty")
	}
	b.tcpRoutes.Add(route)
	return b
}

func (b *ConfigBuilder) validateTCPService(name, strategy string, endpointCount int) {
	if name == "" {
		b.addError("tcp_service name cannot be empty")
	}
	if strategy != "round_robin" && strategy != "weighted_round_robin" {
		b.addError("tcp_service %q has unsupported strategy %q", name, strategy)
	}
	if endpointCount == 0 {
		b.addError("tcp_service %q must have at least one endpoint", name)
	}
}

func (b *ConfigBuilder) validatedTCPEndpoints(name string, endpoints []config.TCPEndpoint) *collectionlist.List[config.TCPEndpoint] {
	endpointList := collectionlist.NewListWithCapacity[config.TCPEndpoint](len(endpoints))
	for _, endpoint := range endpoints {
		endpointList.Add(b.validatedTCPEndpoint(name, endpoint))
	}
	return endpointList
}

func (b *ConfigBuilder) validatedTCPEndpoint(name string, endpoint config.TCPEndpoint) config.TCPEndpoint {
	endpoint.Address = strings.TrimSpace(endpoint.Address)
	if endpoint.Address == "" {
		b.addError("tcp_service %q endpoint address cannot be empty", name)
	} else if _, _, err := net.SplitHostPort(endpoint.Address); err != nil {
		b.addError("tcp_service %q endpoint address %q is invalid", name, endpoint.Address)
	}
	if endpoint.Weight <= 0 {
		endpoint.Weight = 1
	}
	return endpoint
}

func (b *ConfigBuilder) validateTCPRoute(route config.TCPRoute) {
	if route.Name == "" {
		b.addError("tcp_route name cannot be empty")
	}
	if route.Entrypoint == "" {
		b.addError("tcp_route %q entrypoint cannot be empty", route.Name)
	}
	if route.Service == "" {
		b.addError("tcp_route %q service cannot be empty", route.Name)
	}
}

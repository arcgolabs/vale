package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsimple"
)

func Load(path string) (*Config, error) {
	var cfg Config
	if err := hclsimple.DecodeFile(path, nil, &cfg); err != nil {
		return nil, err
	}
	if err := Validate(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Validate(cfg *Config) error {
	if len(cfg.Entrypoints) == 0 {
		return fmt.Errorf("at least one entrypoint is required")
	}
	if len(cfg.Services) == 0 {
		return fmt.Errorf("at least one service is required")
	}
	if len(cfg.Routes) == 0 {
		return fmt.Errorf("at least one route is required")
	}
	engine := strings.TrimSpace(strings.ToLower(cfg.ProxyEngine))
	if engine != "" && engine != "stdlib" && engine != "oxy" {
		return fmt.Errorf("proxy_engine must be one of: stdlib, oxy")
	}

	entrypointSet := make(map[string]struct{}, len(cfg.Entrypoints))
	for _, ep := range cfg.Entrypoints {
		if ep.Name == "" || ep.Address == "" {
			return fmt.Errorf("entrypoint name/address cannot be empty")
		}
		if _, exists := entrypointSet[ep.Name]; exists {
			return fmt.Errorf("duplicated entrypoint %q", ep.Name)
		}
		entrypointSet[ep.Name] = struct{}{}
	}

	serviceSet := make(map[string]struct{}, len(cfg.Services))
	for _, svc := range cfg.Services {
		if svc.Name == "" {
			return fmt.Errorf("service name cannot be empty")
		}
		if _, exists := serviceSet[svc.Name]; exists {
			return fmt.Errorf("duplicated service %q", svc.Name)
		}
		if len(svc.Endpoints) == 0 {
			return fmt.Errorf("service %q must have at least one endpoint", svc.Name)
		}
		for _, endpoint := range svc.Endpoints {
			if endpoint.URL == "" {
				return fmt.Errorf("service %q contains empty endpoint url", svc.Name)
			}
		}
		serviceSet[svc.Name] = struct{}{}
	}

	routeSet := make(map[string]struct{}, len(cfg.Routes))
	for _, route := range cfg.Routes {
		if route.Name == "" {
			return fmt.Errorf("route name cannot be empty")
		}
		if _, exists := routeSet[route.Name]; exists {
			return fmt.Errorf("duplicated route %q", route.Name)
		}
		if _, exists := entrypointSet[route.Entrypoint]; !exists {
			return fmt.Errorf("route %q references unknown entrypoint %q", route.Name, route.Entrypoint)
		}
		if _, exists := serviceSet[route.Service]; !exists {
			return fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
		}
		route.Method = strings.ToUpper(route.Method)
		routeSet[route.Name] = struct{}{}
	}

	return nil
}

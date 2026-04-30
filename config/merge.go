package config

func Merge(configs ...*Config) *Config {
	merged := &Config{
		Entrypoints: make([]Entrypoint, 0),
		Services:    make([]Service, 0),
		Routes:      make([]Route, 0),
	}

	entrypointIndex := make(map[string]int)
	serviceIndex := make(map[string]int)
	routeIndex := make(map[string]int)

	for _, cfg := range configs {
		if cfg == nil {
			continue
		}

		for _, entrypoint := range cfg.Entrypoints {
			if idx, exists := entrypointIndex[entrypoint.Name]; exists {
				merged.Entrypoints[idx] = entrypoint
				continue
			}
			entrypointIndex[entrypoint.Name] = len(merged.Entrypoints)
			merged.Entrypoints = append(merged.Entrypoints, entrypoint)
		}

		for _, service := range cfg.Services {
			if idx, exists := serviceIndex[service.Name]; exists {
				merged.Services[idx] = service
				continue
			}
			serviceIndex[service.Name] = len(merged.Services)
			merged.Services = append(merged.Services, service)
		}

		for _, route := range cfg.Routes {
			if idx, exists := routeIndex[route.Name]; exists {
				merged.Routes[idx] = route
				continue
			}
			routeIndex[route.Name] = len(merged.Routes)
			merged.Routes = append(merged.Routes, route)
		}

		if cfg.ProxyEngine != "" {
			merged.ProxyEngine = cfg.ProxyEngine
		}
		if cfg.Admin != nil {
			admin := *cfg.Admin
			merged.Admin = &admin
		}
		if cfg.Observability != nil {
			observability := *cfg.Observability
			merged.Observability = &observability
		}
		if cfg.Health != nil {
			health := *cfg.Health
			merged.Health = &health
		}
	}

	return merged
}

package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/config"
)

func ConfigEndpoint(rawURL string, weight int) config.Endpoint {
	if weight <= 0 {
		weight = 1
	}
	return config.Endpoint{URL: strings.TrimSpace(rawURL), Weight: weight}
}

func (b *ConfigBuilder) Entrypoint(name, address string, options ...EntrypointOption) *ConfigBuilder {
	if b == nil {
		return nil
	}
	entrypoint := config.Entrypoint{Name: strings.TrimSpace(name), Address: strings.TrimSpace(address)}
	if entrypoint.Name == "" {
		b.addError("entrypoint name cannot be empty")
	}
	if entrypoint.Address == "" {
		b.addError("entrypoint %q address cannot be empty", entrypoint.Name)
	}
	collectionlist.NewList(options...).Range(func(_ int, option EntrypointOption) bool {
		if option != nil {
			option(&entrypoint)
		}
		return true
	})
	b.entrypoints.Add(entrypoint)
	return b
}

func EntrypointTLS(certFile, keyFile string) EntrypointOption {
	return func(entrypoint *config.Entrypoint) {
		if entrypoint == nil {
			return
		}
		entrypoint.TLS = &config.EntrypointTLS{
			Enabled:  true,
			CertFile: strings.TrimSpace(certFile),
			KeyFile:  strings.TrimSpace(keyFile),
		}
	}
}

func EntrypointACME(email, cacheDir string, domains ...string) EntrypointOption {
	return func(entrypoint *config.Entrypoint) {
		if entrypoint == nil {
			return
		}
		domainList := collectionlist.NewListWithCapacity[string](len(domains))
		for _, domain := range domains {
			if trimmed := strings.TrimSpace(domain); trimmed != "" {
				domainList.Add(trimmed)
			}
		}
		entrypoint.ACME = &config.EntrypointACME{
			Enabled:  true,
			Email:    strings.TrimSpace(email),
			CacheDir: strings.TrimSpace(cacheDir),
			Domains:  domainList.Values(),
		}
	}
}

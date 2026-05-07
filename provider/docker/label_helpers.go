package docker

import (
	"strconv"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	"github.com/samber/mo"
)

func newDockerConfig(options Options) *config.Config {
	cfg := provider.NewEntrypointConfig(options.DefaultEntrypointName, options.DefaultEntrypointAddr)
	entrypoints := collectionlist.FilterMapList(collectionlist.NewList(provider.SortedStrings(options.EntrypointAddresses.Keys())...), func(_ int, name string) (config.Entrypoint, bool) {
		if name == options.DefaultEntrypointName {
			return config.Entrypoint{}, false
		}
		address, _ := options.EntrypointAddresses.Get(name)
		address = strings.TrimSpace(address)
		if address == "" {
			return config.Entrypoint{}, false
		}
		return config.Entrypoint{
			Name:    name,
			Address: address,
		}, true
	})
	cfg.Entrypoints = collectionlist.NewList(cfg.Entrypoints...).Merge(entrypoints).Values()
	return cfg
}

func valueOr(value, fallback string) string {
	return mo.EmptyableToOption(strings.TrimSpace(value)).OrElse(fallback)
}

func parseBool(value string, fallback bool) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}

func parseInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	return mo.TupleToOption(parsed, err == nil).OrElse(fallback)
}

func sanitizeName(input, fallback string) string {
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return fallback
	}
	replacer := strings.NewReplacer("/", "-", "_", "-", " ", "-")
	return replacer.Replace(input)
}

func middlewareFromLabels(name string, labels map[string]string) (config.Middleware, bool) {
	middleware := config.Middleware{
		Name:         name,
		StripPrefix:  strings.TrimSpace(labels["vela.middleware.strip_prefix"]),
		AddPrefix:    strings.TrimSpace(labels["vela.middleware.add_prefix"]),
		MaxBodyBytes: int64(parseInt(labels["vela.middleware.max_body_bytes"], 0)),
	}
	return middleware, middleware.StripPrefix != "" || middleware.AddPrefix != "" || middleware.MaxBodyBytes > 0
}

func applyEntrypointTLSLabels(cfg *config.Config, labels map[string]string) {
	if cfg == nil || len(cfg.Entrypoints) == 0 {
		return
	}
	tlsEnabled := parseBool(labels["vela.entrypoint.tls.enabled"], false)
	certFile := strings.TrimSpace(labels["vela.entrypoint.tls.cert_file"])
	keyFile := strings.TrimSpace(labels["vela.entrypoint.tls.key_file"])
	acmeEnabled := parseBool(labels["vela.entrypoint.acme.enabled"], false)
	if tlsEnabled || certFile != "" || keyFile != "" {
		cfg.Entrypoints[0].TLS = &config.EntrypointTLS{
			Enabled:  tlsEnabled,
			CertFile: certFile,
			KeyFile:  keyFile,
		}
	}
	if acmeEnabled {
		cfg.Entrypoints[0].ACME = &config.EntrypointACME{
			Enabled:  true,
			Email:    strings.TrimSpace(labels["vela.entrypoint.acme.email"]),
			CacheDir: strings.TrimSpace(labels["vela.entrypoint.acme.cache_dir"]),
			Domains:  provider.SplitCSV(labels["vela.entrypoint.acme.domains"]),
		}
	}
}

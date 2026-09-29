package compiler

import (
	"strings"

	"github.com/arcgolabs/collectionx/bitset"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/runtime"
)

func compileTLS(entrypoint config.Entrypoint) runtime.TLSRuntime {
	var tlsRuntime runtime.TLSRuntime
	if entrypoint.TLS != nil {
		tlsRuntime.Enabled = entrypoint.TLS.Enabled || entrypoint.TLS.CertFile != "" || entrypoint.TLS.KeyFile != ""
		tlsRuntime.CertFile = strings.TrimSpace(entrypoint.TLS.CertFile)
		tlsRuntime.KeyFile = strings.TrimSpace(entrypoint.TLS.KeyFile)
	}
	if entrypoint.ACME != nil {
		tlsRuntime.Enabled = tlsRuntime.Enabled || entrypoint.ACME.Enabled
		cacheDir := strings.TrimSpace(entrypoint.ACME.CacheDir)
		if entrypoint.ACME.Enabled && cacheDir == "" {
			cacheDir = DefaultACMECacheDir
		}
		tlsRuntime.ACME = runtime.ACMERuntime{
			Enabled:  entrypoint.ACME.Enabled,
			Email:    strings.TrimSpace(entrypoint.ACME.Email),
			CacheDir: cacheDir,
			Domains:  collectionlist.NewList(entrypoint.ACME.Domains...),
		}
	}
	return tlsRuntime
}

func compileRoutePredicates(route config.Route) *bitset.BitSet {
	predicates := bitset.New()
	if strings.TrimSpace(route.Host) != "" {
		predicates.Set(runtime.PredicateHost)
	}
	if strings.TrimSpace(route.PathPrefix) != "" {
		predicates.Set(runtime.PredicatePathPrefix)
	}
	if strings.TrimSpace(route.Method) != "" {
		predicates.Set(runtime.PredicateMethod)
	}
	if len(route.Headers) > 0 {
		predicates.Set(runtime.PredicateHeaders)
	}
	return predicates
}

func pickAdminAddress(cfg *config.Config) string {
	if cfg.Admin != nil && cfg.Admin.Address != "" {
		return cfg.Admin.Address
	}
	return ":19090"
}

func pickAccessLogEnabled(cfg *config.Config) bool {
	if cfg.Observability == nil {
		return true
	}
	return cfg.Observability.AccessLog
}

func pickMetricsEnabled(cfg *config.Config) bool {
	if cfg.Observability == nil {
		return true
	}
	return cfg.Observability.Metrics
}

func pickHealthInterval(cfg *config.Config) string {
	if cfg.Health == nil || cfg.Health.Interval == "" {
		return "5s"
	}
	return cfg.Health.Interval
}

func pickHealthTimeout(cfg *config.Config) string {
	if cfg.Health == nil || cfg.Health.Timeout == "" {
		return "2s"
	}
	return cfg.Health.Timeout
}

func pickSecurity(cfg *config.Config) runtime.SecurityRuntime {
	security := runtime.SecurityRuntime{
		ReadHeaderTimeout: "5s",
		ReadTimeout:       "30s",
		WriteTimeout:      "30s",
		IdleTimeout:       "120s",
		MaxHeaderBytes:    1 << 20,
		MaxBodyBytes:      32 << 20,
	}
	if cfg.Security == nil {
		return security
	}
	if cfg.Security.ReadHeaderTimeout != "" {
		security.ReadHeaderTimeout = cfg.Security.ReadHeaderTimeout
	}
	if cfg.Security.ReadTimeout != "" {
		security.ReadTimeout = cfg.Security.ReadTimeout
	}
	if cfg.Security.WriteTimeout != "" {
		security.WriteTimeout = cfg.Security.WriteTimeout
	}
	if cfg.Security.IdleTimeout != "" {
		security.IdleTimeout = cfg.Security.IdleTimeout
	}
	if cfg.Security.MaxHeaderBytes > 0 {
		security.MaxHeaderBytes = cfg.Security.MaxHeaderBytes
	}
	if cfg.Security.MaxBodyBytes > 0 {
		security.MaxBodyBytes = cfg.Security.MaxBodyBytes
	}
	return security
}

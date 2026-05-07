package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/vela/runtime"
	"github.com/caddyserver/certmagic"
	"github.com/samber/oops"
	"go.uber.org/zap"
)

const defaultACMECacheDir = ".vela/acme"

func (g *Gateway) buildTLSConfig(tlsRuntime runtime.TLSRuntime) (*tls.Config, bool, error) {
	if !tlsRuntime.Enabled {
		return nil, false, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if tlsRuntime.CertFile != "" || tlsRuntime.KeyFile != "" {
		certificate, err := loadStaticTLSCertificate(tlsRuntime)
		if err != nil {
			return nil, false, err
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	if tlsRuntime.ACME.Enabled {
		acmeConfig, err := buildACMETLSConfig(tlsRuntime.ACME)
		if err != nil {
			return nil, false, err
		}
		if len(tlsConfig.Certificates) == 0 {
			tlsConfig.GetCertificate = acmeConfig.GetCertificate
		}
		tlsConfig.NextProtos = mergeTLSNextProtos([]string{"h2", "http/1.1"}, acmeConfig.NextProtos)
	}
	return tlsConfig, true, nil
}

func loadStaticTLSCertificate(tlsRuntime runtime.TLSRuntime) (tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(tlsRuntime.CertFile, tlsRuntime.KeyFile)
	if err != nil {
		return tls.Certificate{}, oops.
			In("gateway").
			With("cert_file", tlsRuntime.CertFile, "key_file", tlsRuntime.KeyFile).
			Wrapf(err, "load static tls certificate")
	}
	return certificate, nil
}

func buildACMETLSConfig(acmeRuntime runtime.ACMERuntime) (*tls.Config, error) {
	if acmeRuntime.Domains == nil || acmeRuntime.Domains.IsEmpty() {
		return nil, oops.
			In("gateway").
			New("acme requires at least one domain")
	}
	cacheDir := strings.TrimSpace(acmeRuntime.CacheDir)
	if cacheDir == "" {
		cacheDir = defaultACMECacheDir
	}

	logger := zap.NewNop()
	cfg := certmagic.NewDefault()
	cfg.Storage = &certmagic.FileStorage{Path: cacheDir}
	cfg.Logger = logger
	cfg.OnDemand = &certmagic.OnDemandConfig{
		DecisionFunc: func(_ context.Context, serverName string) error {
			if acmeDomainAllowed(serverName, acmeRuntime.Domains) {
				return nil
			}
			return fmt.Errorf("acme domain %q is not allowed", serverName)
		},
	}
	cfg.Issuers = []certmagic.Issuer{certmagic.NewACMEIssuer(cfg, certmagic.ACMEIssuer{
		Email:  acmeRuntime.Email,
		Agreed: true,
		Logger: logger,
	})}

	tlsConfig := cfg.TLSConfig()
	tlsConfig.MinVersion = tls.VersionTLS12
	return tlsConfig, nil
}

func acmeDomainAllowed(serverName string, domains *collectionlist.List[string]) bool {
	serverName = strings.ToLower(strings.TrimSpace(serverName))
	if serverName == "" || domains == nil {
		return false
	}
	allowed := false
	domains.Range(func(_ int, domain string) bool {
		domain = strings.ToLower(strings.TrimSpace(domain))
		if domain == "" {
			return true
		}
		if serverName == domain || wildcardDomainMatch(serverName, domain) {
			allowed = true
			return false
		}
		return true
	})
	return allowed
}

func wildcardDomainMatch(serverName, domain string) bool {
	if !strings.HasPrefix(domain, "*.") {
		return false
	}
	suffix := strings.TrimPrefix(domain, "*")
	return strings.HasSuffix(serverName, suffix) && strings.Count(serverName, ".") == strings.Count(domain, ".")
}

func mergeTLSNextProtos(base, extra []string) []string {
	seen := collectionset.NewSet[string]()
	merged := make([]string, 0, len(base)+len(extra))
	for _, proto := range append(base, extra...) {
		if strings.TrimSpace(proto) == "" {
			continue
		}
		if seen.Contains(proto) {
			continue
		}
		seen.Add(proto)
		merged = append(merged, proto)
	}
	return merged
}

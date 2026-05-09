package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/vale/runtime"
	"github.com/caddyserver/certmagic"
	"github.com/samber/oops"
	"go.uber.org/zap"
)

const defaultACMECacheDir = ".vale/acme"

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
		acmeConfig, err := g.buildACMETLSConfig(tlsRuntime.ACME)
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

func (g *Gateway) buildACMETLSConfig(acmeRuntime runtime.ACMERuntime) (*tls.Config, error) {
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
	cfg.Storage = g.acmeStorage(cacheDir)
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

func (g *Gateway) acmeStorage(cacheDir string) certmagic.Storage {
	if g != nil && g.config.CertificateStorage != nil {
		return newCertMagicStorage(g.config.CertificateStorage)
	}
	return &certmagic.FileStorage{Path: cacheDir}
}

func acmeDomainAllowed(serverName string, domains *collectionlist.List[string]) bool {
	serverName = strings.ToLower(strings.TrimSpace(serverName))
	if serverName == "" || domains == nil {
		return false
	}
	return domains.AnyMatch(func(_ int, domain string) bool {
		domain = strings.ToLower(strings.TrimSpace(domain))
		return domain != "" && (serverName == domain || wildcardDomainMatch(serverName, domain))
	})
}

func wildcardDomainMatch(serverName, domain string) bool {
	if !strings.HasPrefix(domain, "*.") {
		return false
	}
	suffix := strings.TrimPrefix(domain, "*")
	return strings.HasSuffix(serverName, suffix) && strings.Count(serverName, ".") == strings.Count(domain, ".")
}

func mergeTLSNextProtos(base, extra []string) []string {
	ordered := collectionset.NewOrderedSet[string]()
	collectionlist.FilterMapList(
		collectionlist.NewList[string]().MergeSlice(base).MergeSlice(extra),
		func(_ int, proto string) (string, bool) {
			trimmed := strings.TrimSpace(proto)
			return trimmed, trimmed != ""
		},
	).Range(func(_ int, proto string) bool {
		ordered.Add(proto)
		return true
	})
	return ordered.Values()
}

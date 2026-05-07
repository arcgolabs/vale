package gateway

import (
	"crypto/tls"

	"github.com/arcgolabs/vela/runtime"
	"github.com/samber/oops"
	"golang.org/x/crypto/acme/autocert"
)

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
		applyACMETLSConfig(tlsConfig, tlsRuntime.ACME)
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

func applyACMETLSConfig(tlsConfig *tls.Config, acmeRuntime runtime.ACMERuntime) {
	manager := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Email:  acmeRuntime.Email,
	}
	if acmeRuntime.Domains != nil && !acmeRuntime.Domains.IsEmpty() {
		manager.HostPolicy = autocert.HostWhitelist(acmeRuntime.Domains.Values()...)
	}
	if acmeRuntime.CacheDir != "" {
		manager.Cache = autocert.DirCache(acmeRuntime.CacheDir)
	}
	acmeConfig := manager.TLSConfig()
	if len(tlsConfig.Certificates) == 0 {
		tlsConfig.GetCertificate = acmeConfig.GetCertificate
	}
	tlsConfig.NextProtos = acmeConfig.NextProtos
}

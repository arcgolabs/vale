package config

import (
	"strings"
	"testing"
)

func TestValidateReportsUnknownRouteReferences(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Entrypoints: []Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []Service{{
			Name: "api",
			Endpoints: []Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Routes: []Route{{
			Name:       "missing-service",
			Entrypoint: "web",
			Service:    "unknown",
		}},
	}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `unknown service "unknown"`) {
		t.Fatalf("Validate error = %v, want unknown service", err)
	}

	cfg.Routes[0].Entrypoint = "unknown"
	cfg.Routes[0].Service = "api"
	err = Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `unknown entrypoint "unknown"`) {
		t.Fatalf("Validate error = %v, want unknown entrypoint", err)
	}
}

func TestValidateReportsUnknownMiddleware(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Entrypoints: []Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []Service{{
			Name: "api",
			Endpoints: []Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Routes: []Route{{
			Name:        "api",
			Entrypoint:  "web",
			Service:     "api",
			Middlewares: []string{"missing"},
		}},
	}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `unknown middleware "missing"`) {
		t.Fatalf("Validate error = %v, want unknown middleware", err)
	}
}

func TestValidateEntrypointTLSAndACME(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		Entrypoints: []Entrypoint{{
			Name:    "websecure",
			Address: ":8443",
			TLS: &EntrypointTLS{
				Enabled:  true,
				CertFile: "cert.pem",
			},
		}},
		Services: []Service{{
			Name: "api",
			Endpoints: []Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Routes: []Route{{
			Name:       "api",
			Entrypoint: "websecure",
			Service:    "api",
		}},
	}

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "tls requires both") {
		t.Fatalf("Validate error = %v, want tls pair error", err)
	}

	cfg.Entrypoints[0].TLS.KeyFile = "key.pem"
	cfg.Entrypoints[0].ACME = &EntrypointACME{Enabled: true}
	err = Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "acme requires at least one domain") {
		t.Fatalf("Validate error = %v, want acme domains error", err)
	}
}

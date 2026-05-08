package config_test

import (
	"strings"
	"testing"

	"github.com/arcgolabs/vale/config"
)

func TestValidateReportsUnknownRouteReferences(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []config.Service{{
			Name: "api",
			Endpoints: []config.Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Routes: []config.Route{{
			Name:       "missing-service",
			Entrypoint: "web",
			Service:    "unknown",
		}},
	}

	err := config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `unknown service "unknown"`) {
		t.Fatalf("Validate error = %v, want unknown service", err)
	}

	cfg.Routes[0].Entrypoint = "unknown"
	cfg.Routes[0].Service = "api"
	err = config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `unknown entrypoint "unknown"`) {
		t.Fatalf("Validate error = %v, want unknown entrypoint", err)
	}
}

func TestValidateReportsUnknownMiddleware(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []config.Service{{
			Name: "api",
			Endpoints: []config.Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Routes: []config.Route{{
			Name:        "api",
			Entrypoint:  "web",
			Service:     "api",
			Middlewares: []string{"missing"},
		}},
	}

	err := config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), `unknown middleware "missing"`) {
		t.Fatalf("Validate error = %v, want unknown middleware", err)
	}
}

func TestValidateEntrypointTLSAndACME(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{
			Name:    "websecure",
			Address: ":8443",
			TLS: &config.EntrypointTLS{
				Enabled:  true,
				CertFile: "cert.pem",
			},
		}},
		Services: []config.Service{{
			Name: "api",
			Endpoints: []config.Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Routes: []config.Route{{
			Name:       "api",
			Entrypoint: "websecure",
			Service:    "api",
		}},
	}

	err := config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "tls requires both") {
		t.Fatalf("Validate error = %v, want tls pair error", err)
	}

	cfg.Entrypoints[0].TLS.KeyFile = "key.pem"
	cfg.Entrypoints[0].ACME = &config.EntrypointACME{Enabled: true}
	err = config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "acme requires at least one domain") {
		t.Fatalf("Validate error = %v, want acme domains error", err)
	}

	cfg.Entrypoints[0].ACME.Domains = []string{"example.com"}
	err = config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "acme requires email") {
		t.Fatalf("Validate error = %v, want acme email error", err)
	}
}

func TestValidateMiddlewarePolicyOptions(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []config.Service{{
			Name: "api",
			Endpoints: []config.Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Middlewares: []config.Middleware{{
			Name:      "limited",
			RateLimit: &config.RateLimit{Rate: -1},
		}},
		Routes: []config.Route{{
			Name:        "api",
			Entrypoint:  "web",
			Service:     "api",
			Middlewares: []string{"limited"},
		}},
	}

	err := config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "rate_limit rate") {
		t.Fatalf("Validate error = %v, want rate limit error", err)
	}

	cfg.Middlewares[0].RateLimit = nil
	cfg.Middlewares[0].Secure = &config.SecureMiddleware{STSSeconds: -1}
	err = config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "sts_seconds") {
		t.Fatalf("Validate error = %v, want secure error", err)
	}
}

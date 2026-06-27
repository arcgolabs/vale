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

	cfg.Middlewares[0].Secure = nil
	cfg.Middlewares[0].ForwardAuth = &config.ForwardAuth{Address: "ftp://auth.local"}
	err = config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "scheme") {
		t.Fatalf("Validate error = %v, want forward auth scheme error", err)
	}

	cfg.Middlewares[0].ForwardAuth = &config.ForwardAuth{Address: "http://auth.local/validate", Timeout: "-1s"}
	err = config.Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("Validate error = %v, want forward auth timeout error", err)
	}
}

func TestValidateTCPConfigAndProtocolBoundaries(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "db", Address: ":15432", Protocol: "tcp"}},
		TCPServices: []config.TCPService{{
			Name:      "postgres",
			Endpoints: []config.TCPEndpoint{{Address: "127.0.0.1:5432"}},
		}},
		TCPRoutes: []config.TCPRoute{{Name: "postgres", Entrypoint: "db", Service: "postgres"}},
	}
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("Validate tcp config error = %v", err)
	}

	cfg.Routes = []config.Route{{Name: "http-on-tcp", Entrypoint: "db", Service: "api"}}
	cfg.Services = []config.Service{{Name: "api", Endpoints: []config.Endpoint{{URL: "http://127.0.0.1:8081"}}}}
	if err := config.Validate(cfg); err == nil || !strings.Contains(err.Error(), "references tcp entrypoint") {
		t.Fatalf("Validate error = %v, want http route on tcp entrypoint error", err)
	}

	cfg = &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "web", Address: ":8080"}},
		TCPServices: []config.TCPService{{
			Name:      "postgres",
			Endpoints: []config.TCPEndpoint{{Address: "127.0.0.1:5432"}},
		}},
		TCPRoutes: []config.TCPRoute{{Name: "postgres", Entrypoint: "web", Service: "postgres"}},
	}
	if err := config.Validate(cfg); err == nil || !strings.Contains(err.Error(), "references non-tcp entrypoint") {
		t.Fatalf("Validate error = %v, want tcp route on http entrypoint error", err)
	}
}

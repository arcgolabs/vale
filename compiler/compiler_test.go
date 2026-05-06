package compiler

import (
	"testing"

	"github.com/arcgolabs/vela/config"
)

func TestCompileTLSMiddlewareAndSecurity(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{
			Name:    "websecure",
			Address: ":8443",
			TLS: &config.EntrypointTLS{
				CertFile: "cert.pem",
				KeyFile:  "key.pem",
			},
			ACME: &config.EntrypointACME{
				Enabled:  true,
				Email:    "ops@example.com",
				CacheDir: "./acme",
				Domains:  []string{"example.com"},
			},
		}},
		Services: []config.Service{{
			Name: "api",
			Endpoints: []config.Endpoint{
				{URL: "http://127.0.0.1:8081"},
			},
		}},
		Middlewares: []config.Middleware{{
			Name:         "strip-api",
			StripPrefix:  "/api",
			MaxBodyBytes: 1024,
		}},
		Routes: []config.Route{{
			Name:        "api",
			Entrypoint:  "websecure",
			Service:     "api",
			PathPrefix:  "/api",
			Middlewares: []string{"strip-api"},
		}},
		Security: &config.Security{
			ReadHeaderTimeout: "3s",
			MaxHeaderBytes:    2048,
			MaxBodyBytes:      4096,
		},
	}

	snapshot, err := Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}

	entrypoint := snapshot.EntrypointConfigs["websecure"]
	if !entrypoint.TLS.Enabled || entrypoint.TLS.CertFile != "cert.pem" || !entrypoint.TLS.ACME.Enabled {
		t.Fatalf("unexpected tls runtime: %#v", entrypoint.TLS)
	}
	routes := snapshot.RoutesByEntrypoint["websecure"]
	if len(routes) != 1 || len(routes[0].Middlewares) != 1 {
		t.Fatalf("compiled middlewares = %#v", routes)
	}
	if routes[0].Middlewares[0].StripPrefix != "/api" {
		t.Fatalf("middleware = %#v", routes[0].Middlewares[0])
	}
	if snapshot.Security.ReadHeaderTimeout != "3s" || snapshot.Security.MaxHeaderBytes != 2048 || snapshot.Security.MaxBodyBytes != 4096 {
		t.Fatalf("security = %#v", snapshot.Security)
	}
}

package compiler

import (
	"strings"
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

	entrypoint, _ := snapshot.EntrypointConfigs.Get("websecure")
	if !entrypoint.TLS.Enabled || entrypoint.TLS.CertFile != "cert.pem" || !entrypoint.TLS.ACME.Enabled {
		t.Fatalf("unexpected tls runtime: %#v", entrypoint.TLS)
	}
	routes := snapshot.RoutesByEntrypoint.Get("websecure")
	if len(routes) != 1 || routes[0].Middlewares.Len() != 1 {
		t.Fatalf("compiled middlewares = %#v", routes)
	}
	middleware, _ := routes[0].Middlewares.Get(0)
	if middleware.StripPrefix != "/api" {
		t.Fatalf("middleware = %#v", middleware)
	}
	if snapshot.Security.ReadHeaderTimeout != "3s" || snapshot.Security.MaxHeaderBytes != 2048 || snapshot.Security.MaxBodyBytes != 4096 {
		t.Fatalf("security = %#v", snapshot.Security)
	}
}

func TestCompileACMEAppliesDefaultCacheDir(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{
			Name:    "websecure",
			Address: ":8443",
			ACME: &config.EntrypointACME{
				Enabled: true,
				Email:   "ops@example.com",
				Domains: []string{"example.com"},
			},
		}},
		Services: []config.Service{{
			Name:      "api",
			Endpoints: []config.Endpoint{{URL: "http://127.0.0.1:8081"}},
		}},
		Routes: []config.Route{{
			Name:       "api",
			Entrypoint: "websecure",
			Service:    "api",
		}},
	}

	snapshot, err := Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	entrypoint, _ := snapshot.EntrypointConfigs.Get("websecure")
	if entrypoint.TLS.ACME.CacheDir != DefaultACMECacheDir {
		t.Fatalf("acme cache dir = %q, want %q", entrypoint.TLS.ACME.CacheDir, DefaultACMECacheDir)
	}
}

func TestCompileRejectsUnknownMiddlewareType(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []config.Service{{
			Name:      "api",
			Endpoints: []config.Endpoint{{URL: "http://127.0.0.1:8081"}},
		}},
		Middlewares: []config.Middleware{{
			Name: "custom-auth",
			Type: "custom",
		}},
		Routes: []config.Route{{
			Name:        "api",
			Entrypoint:  "web",
			Service:     "api",
			Middlewares: []string{"custom-auth"},
		}},
	}

	_, err := Compile(cfg)
	if err == nil || !strings.Contains(err.Error(), `unsupported type "custom"`) {
		t.Fatalf("Compile error = %v, want unsupported middleware type", err)
	}
}

package compiler_test

import (
	"strings"
	"testing"

	"github.com/arcgolabs/vale/compiler"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/runtime"
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
			BasicAuth: &config.BasicAuth{
				Realm: "private",
				Users: map[string]string{"admin": "secret"},
			},
			Compress: &config.Compress{
				Enabled:  true,
				MinBytes: 128,
			},
			IPAllowList: &config.IPAllowList{
				SourceRange:        []string{"127.0.0.1"},
				TrustForwardHeader: true,
			},
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

	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}

	assertCompiledTLS(t, snapshot)
	assertCompiledMiddleware(t, snapshot)
	assertCompiledSecurity(t, snapshot)
}

func assertCompiledTLS(t *testing.T, snapshot *runtime.CompiledSnapshot) {
	t.Helper()
	entrypoint, _ := snapshot.EntrypointConfigs.Get("websecure")
	if !entrypoint.TLS.Enabled || entrypoint.TLS.CertFile != "cert.pem" || !entrypoint.TLS.ACME.Enabled {
		t.Fatalf("unexpected tls runtime: %#v", entrypoint.TLS)
	}
}

func assertCompiledMiddleware(t *testing.T, snapshot *runtime.CompiledSnapshot) {
	t.Helper()
	routes := snapshot.RoutesByEntrypoint.Get("websecure")
	if len(routes) != 1 || routes[0].Middlewares.Len() != 1 {
		t.Fatalf("compiled middlewares = %#v", routes)
	}
	middleware, _ := routes[0].Middlewares.Get(0)
	if middleware.StripPrefix != "/api" {
		t.Fatalf("middleware = %#v", middleware)
	}
	if !middleware.BasicAuth.Enabled || middleware.BasicAuth.Users == nil {
		t.Fatalf("basic auth middleware = %#v", middleware.BasicAuth)
	}
	if !middleware.Compress.Enabled || middleware.Compress.MinBytes != 128 {
		t.Fatalf("compress middleware = %#v", middleware.Compress)
	}
	if !middleware.IPAllowList.Enabled || !middleware.IPAllowList.TrustForwardHeader {
		t.Fatalf("ip allow list middleware = %#v", middleware.IPAllowList)
	}
}

func assertCompiledSecurity(t *testing.T, snapshot *runtime.CompiledSnapshot) {
	t.Helper()
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

	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	entrypoint, _ := snapshot.EntrypointConfigs.Get("websecure")
	if entrypoint.TLS.ACME.CacheDir != compiler.DefaultACMECacheDir {
		t.Fatalf("acme cache dir = %q, want %q", entrypoint.TLS.ACME.CacheDir, compiler.DefaultACMECacheDir)
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

	_, err := compiler.Compile(cfg)
	if err == nil || !strings.Contains(err.Error(), `unsupported type "custom"`) {
		t.Fatalf("Compile error = %v, want unsupported middleware type", err)
	}
}

func TestCompileMiddlewareChainExpandsBuiltins(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Entrypoints: []config.Entrypoint{{Name: "web", Address: ":8080"}},
		Services: []config.Service{{
			Name:      "api",
			Endpoints: []config.Endpoint{{URL: "http://127.0.0.1:8081"}},
		}},
		Middlewares: []config.Middleware{
			{
				Name:          "strip",
				Type:          "strip_prefix",
				StripPrefixes: []string{"/api", " /v1 "},
			},
			{
				Name:              "redirect",
				Type:              "redirect_scheme",
				RedirectScheme:    "https",
				RedirectPort:      "443",
				RedirectPermanent: true,
			},
			{
				Name:  "chain",
				Type:  "chain",
				Chain: []string{"strip", "redirect"},
			},
		},
		Routes: []config.Route{{
			Name:        "api",
			Entrypoint:  "web",
			Service:     "api",
			Middlewares: []string{"chain"},
		}},
	}

	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	routes := snapshot.RoutesByEntrypoint.Get("web")
	if len(routes) != 1 || routes[0].Middlewares.Len() != 2 {
		t.Fatalf("compiled middlewares = %#v", routes)
	}
	strip, _ := routes[0].Middlewares.Get(0)
	if strip.Type != "builtin" || strip.StripPrefixes.Len() != 2 {
		t.Fatalf("strip middleware = %#v", strip)
	}
	redirect, _ := routes[0].Middlewares.Get(1)
	if redirect.Type != "builtin" || redirect.RedirectScheme != "https" || !redirect.RedirectPermanent {
		t.Fatalf("redirect middleware = %#v", redirect)
	}
}

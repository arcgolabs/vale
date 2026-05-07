package provider_test

import (
	"net/http"
	"testing"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/provider"
)

func TestParseTraefikLabelsProjectsHTTPResources(t *testing.T) {
	t.Parallel()

	labels := provider.ParseTraefikLabels(mapping.NewMapFrom(map[string]string{
		"Traefik.Enable":                                                              "true",
		"traefik.http.routers.api.rule":                                               "Host(`api.example.com`) && PathPrefix(`/api`) && Method(`" + http.MethodGet + "`) && Headers(`X-Tenant`, `acme`)",
		"traefik.http.routers.api.entrypoints":                                        "web,websecure",
		"traefik.http.routers.api.middlewares":                                        "strip@docker,headers",
		"traefik.http.routers.api.service":                                            "api-svc@docker",
		"traefik.http.services.api-svc.loadbalancer.server.port":                      "8081",
		"traefik.http.services.api-svc.loadbalancer.server.scheme":                    "https",
		"traefik.http.middlewares.strip.stripprefix.prefixes":                         "/api,/v1",
		"traefik.http.middlewares.headers.headers.customrequestheaders.x-request-id":  "from-label",
		"traefik.http.middlewares.headers.headers.customresponseheaders.x-powered-by": "vela",
		"traefik.http.middlewares.rewrite.replacepathregex.regex":                     "^/old/(.*)$",
		"traefik.http.middlewares.rewrite.replacepathregex.replacement":               "/new/${1}",
		"traefik.http.middlewares.redirect.redirectscheme.scheme":                     "https",
		"traefik.http.middlewares.redirect.redirectscheme.port":                       "443",
		"traefik.http.middlewares.redirect.redirectscheme.permanent":                  "true",
		"traefik.http.middlewares.chain.chain.middlewares":                            "strip@docker,headers,redirect",
		"traefik.http.middlewares.security.headers.framedeny":                         "true",
		"traefik.http.middlewares.security.headers.contenttypenosniff":                "true",
		"traefik.http.middlewares.security.headers.stsseconds":                        "31536000",
	}))

	assertTraefikRouter(t, labels)
	assertTraefikService(t, labels)
	assertTraefikMiddlewares(t, labels)
}

func assertTraefikRouter(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	if enabled := labels.Enabled.OrElse(false); !enabled {
		t.Fatal("enabled = false, want true")
	}
	router, ok := labels.Routers.Get("api")
	if !ok {
		t.Fatal("router api was not parsed")
	}
	if router.Host != "api.example.com" || router.PathPrefix != "/api" || router.Method != http.MethodGet {
		t.Fatalf("router = %#v", router)
	}
	header, _ := router.Headers.Get("X-Tenant")
	if header != "acme" {
		t.Fatalf("header = %q, want acme", header)
	}
	if router.Entrypoints.Values()[1] != "websecure" {
		t.Fatalf("entrypoints = %v", router.Entrypoints.Values())
	}
	if router.Middlewares.Values()[0] != "strip" || router.Service != "api-svc" {
		t.Fatalf("middleware/service = %v/%q", router.Middlewares.Values(), router.Service)
	}
}

func assertTraefikService(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	service, ok := labels.Services.Get("api-svc")
	if !ok {
		t.Fatal("service api-svc was not parsed")
	}
	if service.Port != 8081 || service.Scheme != "https" {
		t.Fatalf("service = %#v", service)
	}
}

func assertTraefikMiddlewares(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	assertTraefikStripMiddleware(t, labels)
	assertTraefikHeaderMiddleware(t, labels)
	assertTraefikRewriteMiddleware(t, labels)
	assertTraefikRedirectMiddleware(t, labels)
	assertTraefikChainMiddleware(t, labels)
	assertTraefikSecurityMiddleware(t, labels)
}

func assertTraefikStripMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	middleware, _ := labels.Middlewares.Get("strip")
	if middleware.StripPrefix != "/api" || len(middleware.StripPrefixes) != 2 {
		t.Fatalf("strip middleware = %#v", middleware)
	}
}

func assertTraefikHeaderMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	middleware, _ := labels.Middlewares.Get("headers")
	if middleware.RequestHeaders["x-request-id"] != "from-label" || middleware.ResponseHeaders["x-powered-by"] != "vela" {
		t.Fatalf("headers = %#v", middleware)
	}
}

func assertTraefikRewriteMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	middleware, _ := labels.Middlewares.Get("rewrite")
	if middleware.ReplacePathRegex != "^/old/(.*)$" || middleware.ReplacePathReplacement != "/new/${1}" {
		t.Fatalf("rewrite = %#v", middleware)
	}
}

func assertTraefikRedirectMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	middleware, _ := labels.Middlewares.Get("redirect")
	if middleware.RedirectScheme != "https" || middleware.RedirectPort != "443" || !middleware.RedirectPermanent {
		t.Fatalf("redirect = %#v", middleware)
	}
}

func assertTraefikChainMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	middleware, _ := labels.Middlewares.Get("chain")
	if len(middleware.Chain) != 3 || middleware.Chain[0] != "strip" || middleware.Chain[2] != "redirect" {
		t.Fatalf("chain = %#v", middleware.Chain)
	}
}

func assertTraefikSecurityMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	security, _ := labels.Middlewares.Get("security")
	if security.ResponseHeaders["x-frame-options"] != "DENY" ||
		security.ResponseHeaders["x-content-type-options"] != "nosniff" ||
		security.ResponseHeaders["strict-transport-security"] != "max-age=31536000" {
		t.Fatalf("security headers = %#v", security.ResponseHeaders)
	}
}

func TestParseTraefikLabelsRecognizesHTTPConfigWithoutEnable(t *testing.T) {
	t.Parallel()

	labels := provider.ParseTraefikLabels(mapping.NewMapFrom(map[string]string{
		"traefik.http.routers.web.rule": "Path(`/`)",
	}))
	if !labels.HasHTTPConfig() {
		t.Fatal("HasHTTPConfig = false, want true")
	}
	if labels.Enabled.IsPresent() {
		t.Fatal("enabled option was present")
	}
}

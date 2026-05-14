package provider_test

import (
	"net/http"
	"testing"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/provider"
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
		"traefik.http.middlewares.headers.headers.customresponseheaders.x-powered-by": "vale",
		"traefik.http.middlewares.rewrite.replacepathregex.regex":                     "^/old/(.*)$",
		"traefik.http.middlewares.rewrite.replacepathregex.replacement":               "/new/${1}",
		"traefik.http.middlewares.redirect.redirectscheme.scheme":                     "https",
		"traefik.http.middlewares.redirect.redirectscheme.port":                       "443",
		"traefik.http.middlewares.redirect.redirectscheme.permanent":                  "true",
		"traefik.http.middlewares.chain.chain.middlewares":                            "strip@docker,headers,redirect",
		"traefik.http.middlewares.auth.basicauth.realm":                               "private",
		"traefik.http.middlewares.auth.basicauth.users":                               "admin:secret,ops:deploy",
		"traefik.http.middlewares.compress.compress":                                  "true",
		"traefik.http.middlewares.compress.compress.minresponsebodybytes":             "128",
		"traefik.http.middlewares.allow.ipallowlist.sourcerange":                      "127.0.0.1,10.0.0.0/8",
		"traefik.http.middlewares.allow.ipallowlist.trustforwardheader":               "true",
		"traefik.http.middlewares.limit.ratelimit.average":                            "10",
		"traefik.http.middlewares.limit.ratelimit.burst":                              "20",
		"traefik.http.middlewares.forward.forwardauth.address":                        "http://auth.local/validate",
		"traefik.http.middlewares.forward.forwardauth.trustforwardheader":             "true",
		"traefik.http.middlewares.forward.forwardauth.authrequestheaders":             "Authorization,X-Request-ID",
		"traefik.http.middlewares.forward.forwardauth.authresponseheaders":            "X-Authenticated-Subject",
		"traefik.http.middlewares.forward.forwardauth.forwardbody":                    "true",
		"traefik.http.middlewares.forward.forwardauth.maxbodysize":                    "4096",
		"traefik.http.middlewares.forward.forwardauth.maxresponsebodysize":            "2048",
		"traefik.http.middlewares.forward.forwardauth.timeout":                        "750ms",
		"traefik.http.middlewares.cors.headers.accesscontrolalloworiginlist":          "https://ui.example.com,https://admin.example.com",
		"traefik.http.middlewares.cors.headers.accesscontrolallowmethods":             "GET,POST",
		"traefik.http.middlewares.cors.headers.accesscontrolallowcredentials":         "true",
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
	assertTraefikPolicyMiddlewares(t, labels)
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
	if middleware.RequestHeaders["x-request-id"] != "from-label" || middleware.ResponseHeaders["x-powered-by"] != "vale" {
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

func assertTraefikPolicyMiddlewares(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	assertTraefikBasicAuthMiddleware(t, labels)
	assertTraefikCompressMiddleware(t, labels)
	assertTraefikIPAllowListMiddleware(t, labels)
	assertTraefikRateLimitMiddleware(t, labels)
	assertTraefikForwardAuthMiddleware(t, labels)
	assertTraefikCORSMiddleware(t, labels)
}

func assertTraefikBasicAuthMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	auth, _ := labels.Middlewares.Get("auth")
	if auth.BasicAuth == nil || auth.BasicAuth.Realm != "private" || auth.BasicAuth.Users["admin"] != "secret" {
		t.Fatalf("basic auth = %#v", auth.BasicAuth)
	}
}

func assertTraefikCompressMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	compress, _ := labels.Middlewares.Get("compress")
	if compress.Compress == nil || !compress.Compress.Enabled || compress.Compress.MinBytes != 128 {
		t.Fatalf("compress = %#v", compress.Compress)
	}
}

func assertTraefikIPAllowListMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	allow, _ := labels.Middlewares.Get("allow")
	if allow.IPAllowList == nil || !allow.IPAllowList.TrustForwardHeader || len(allow.IPAllowList.SourceRange) != 2 {
		t.Fatalf("ip allow list = %#v", allow.IPAllowList)
	}
}

func assertTraefikRateLimitMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	limit, _ := labels.Middlewares.Get("limit")
	if limit.RateLimit == nil || limit.RateLimit.Rate != 10 || limit.RateLimit.Burst != 20 {
		t.Fatalf("rate limit = %#v", limit.RateLimit)
	}
}

func assertTraefikForwardAuthMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	forward, _ := labels.Middlewares.Get("forward")
	if forward.ForwardAuth == nil ||
		forward.ForwardAuth.Address != "http://auth.local/validate" ||
		!forward.ForwardAuth.TrustForwardHeader ||
		!forward.ForwardAuth.ForwardBody ||
		forward.ForwardAuth.MaxBodyBytes != 4096 ||
		forward.ForwardAuth.MaxResponseBodyBytes != 2048 ||
		forward.ForwardAuth.Timeout != "750ms" ||
		len(forward.ForwardAuth.AuthRequestHeaders) != 2 ||
		forward.ForwardAuth.AuthResponseHeaders[0] != "X-Authenticated-Subject" {
		t.Fatalf("forward auth = %#v", forward.ForwardAuth)
	}
}

func assertTraefikCORSMiddleware(t *testing.T, labels provider.TraefikLabels) {
	t.Helper()
	cors, _ := labels.Middlewares.Get("cors")
	if cors.CORS == nil || len(cors.CORS.AllowedOrigins) != 2 || !cors.CORS.AllowCredentials {
		t.Fatalf("cors = %#v", cors.CORS)
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

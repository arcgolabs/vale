package provider_test

import (
	"net/http"
	"testing"

	"github.com/arcgolabs/vela/provider"
)

func TestParseTraefikLabelsProjectsHTTPResources(t *testing.T) {
	t.Parallel()

	labels := provider.ParseTraefikLabels(map[string]string{
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
	})

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
	strip, _ := labels.Middlewares.Get("strip")
	if strip.StripPrefix != "/api" {
		t.Fatalf("strip prefix = %q, want /api", strip.StripPrefix)
	}
	headers, _ := labels.Middlewares.Get("headers")
	if headers.RequestHeaders["x-request-id"] != "from-label" || headers.ResponseHeaders["x-powered-by"] != "vela" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestParseTraefikLabelsRecognizesHTTPConfigWithoutEnable(t *testing.T) {
	t.Parallel()

	labels := provider.ParseTraefikLabels(map[string]string{
		"traefik.http.routers.web.rule": "Path(`/`)",
	})
	if !labels.HasHTTPConfig() {
		t.Fatal("HasHTTPConfig = false, want true")
	}
	if labels.Enabled.IsPresent() {
		t.Fatal("enabled option was present")
	}
}

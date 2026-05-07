package docker_test

import (
	"testing"

	"github.com/arcgolabs/collectionx/mapping"
	providerdocker "github.com/arcgolabs/vela/provider/docker"
)

func TestProviderLoadsTraefikLabels(t *testing.T) {
	t.Parallel()

	source := providerdocker.NewMemorySource(providerdocker.Container{
		Name:    "api",
		Address: "10.0.0.2",
		Port:    80,
		Labels: mapping.NewMapFrom(map[string]string{
			"traefik.http.routers.api.rule":                                "Host(`api.example.com`) && PathPrefix(`/api`)",
			"traefik.http.routers.api.entrypoints":                         "web,websecure",
			"traefik.http.routers.api.middlewares":                         "strip@docker",
			"traefik.http.routers.api.service":                             "api-service",
			"traefik.http.services.api-service.loadbalancer.server.port":   "8081",
			"traefik.http.services.api-service.loadbalancer.server.scheme": "https",
			"traefik.http.middlewares.strip.stripprefix.prefixes":          "/api,/v1",
		}),
	})
	provider := providerdocker.New("docker", source, providerdocker.Options{
		DefaultEntrypointName: "web",
		DefaultEntrypointAddr: ":8080",
		EntrypointAddresses: mapping.NewMapFrom(map[string]string{
			"web":       ":8080",
			"websecure": ":8443",
		}),
	})

	cfg, err := provider.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Services) != 1 || cfg.Services[0].Endpoints[0].URL != "https://10.0.0.2:8081" {
		t.Fatalf("services = %#v", cfg.Services)
	}
	if len(cfg.Middlewares) != 1 || cfg.Middlewares[0].StripPrefix != "/api" {
		t.Fatalf("middlewares = %#v", cfg.Middlewares)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("routes len = %d, want 2", len(cfg.Routes))
	}
	if cfg.Routes[0].Host != "api.example.com" || cfg.Routes[0].PathPrefix != "/api" {
		t.Fatalf("route = %#v", cfg.Routes[0])
	}
}

func TestProviderFallsBackToVelaLabels(t *testing.T) {
	t.Parallel()

	source := providerdocker.NewMemorySource(providerdocker.Container{
		Name:    "api",
		Address: "10.0.0.2",
		Port:    8080,
		Labels: mapping.NewMapFrom(map[string]string{
			"vela.enable":          "true",
			"vela.service":         "api",
			"vela.route":           "api-route",
			"vela.rule.pathprefix": "/api",
		}),
	})
	provider := providerdocker.New("docker", source, providerdocker.DefaultOptions())

	cfg, err := provider.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Name != "api-route" {
		t.Fatalf("routes = %#v", cfg.Routes)
	}
}

func TestProviderDisablesExplicitTraefikFalse(t *testing.T) {
	t.Parallel()

	source := providerdocker.NewMemorySource(providerdocker.Container{
		Name:    "api",
		Address: "10.0.0.2",
		Port:    8080,
		Labels: mapping.NewMapFrom(map[string]string{
			"traefik.enable":                "false",
			"traefik.http.routers.api.rule": "PathPrefix(`/api`)",
		}),
	})
	provider := providerdocker.New("docker", source, providerdocker.DefaultOptions())

	cfg, err := provider.Load(t.Context())
	if err == nil {
		t.Fatalf("Load returned nil error with no enabled routes: %#v", cfg)
	}
}

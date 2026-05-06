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

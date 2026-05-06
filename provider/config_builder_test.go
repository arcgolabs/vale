package provider

import (
	"reflect"
	"testing"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/config"
)

func TestAppendSortedServices(t *testing.T) {
	cfg := NewEntrypointConfig("web", ":8080")
	services := mapping.NewMap[string, *config.Service]()
	services.Set("b", &config.Service{Name: "b"})
	services.Set("a", &config.Service{Name: "a"})

	AppendSortedServices(cfg, services)

	got := []string{cfg.Services[0].Name, cfg.Services[1].Name}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("services = %v", got)
	}
}

func TestAppendSortedRoutes(t *testing.T) {
	cfg := NewEntrypointConfig("web", ":8080")
	routes := mapping.NewMap[string, config.Route]()
	routes.Set("b", config.Route{Name: "b"})
	routes.Set("a", config.Route{Name: "a"})

	AppendSortedRoutes(cfg, routes)

	got := []string{cfg.Routes[0].Name, cfg.Routes[1].Name}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("routes = %v", got)
	}
}

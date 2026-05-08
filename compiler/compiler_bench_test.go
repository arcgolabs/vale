package compiler_test

import (
	"fmt"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/compiler"
	"github.com/arcgolabs/vale/config"
)

func BenchmarkCompileByRouteCount(b *testing.B) {
	for _, routeCount := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("routes_%d", routeCount), benchmarkCompileByRouteCount(routeCount))
	}
}

func benchmarkCompileByRouteCount(routeCount int) func(*testing.B) {
	return func(b *testing.B) {
		cfg := benchmarkConfig(routeCount)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			snapshot, err := compiler.Compile(cfg)
			if err != nil {
				b.Fatal(err)
			}
			if snapshot.Routes().Len() != routeCount {
				b.Fatalf("routes = %d, want %d", snapshot.Routes().Len(), routeCount)
			}
		}
	}
}

func benchmarkConfig(routeCount int) *config.Config {
	routes := collectionlist.NewListWithCapacity[config.Route](routeCount)
	for i := range routeCount {
		routes.Add(config.Route{
			Name:       fmt.Sprintf("api-%04d", i),
			Entrypoint: "web",
			Service:    "api",
			Host:       "api.example.com",
			PathPrefix: fmt.Sprintf("/api/%04d", i),
			Method:     "GET",
		})
	}
	return &config.Config{
		Entrypoints: []config.Entrypoint{
			{Name: "web", Address: ":8080"},
		},
		Services: []config.Service{
			{
				Name:     "api",
				Strategy: "round_robin",
				Endpoints: []config.Endpoint{
					{URL: "http://127.0.0.1:8081", Weight: 1},
				},
			},
		},
		Routes: routes.Values(),
		Admin:  &config.Admin{Address: ":19090"},
		Observability: &config.Observability{
			AccessLog: false,
			Metrics:   false,
		},
	}
}

package vela_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
)

func TestRegistryUseRegistersCompileTimePlugin(t *testing.T) {
	t.Parallel()

	registry := vela.NewRegistry()
	plugin := vela.PluginFunc(func(registry *vela.Registry) error {
		if err := registry.RegisterConfigProvider("memory", newTestPluginConfigProvider); err != nil {
			return fmt.Errorf("register config provider: %w", err)
		}
		if err := registry.RegisterMiddleware("mark", markMiddleware); err != nil {
			return fmt.Errorf("register middleware: %w", err)
		}
		if err := registry.RegisterMetricsFactory("noop", newNoopPluginMetrics); err != nil {
			return fmt.Errorf("register metrics: %w", err)
		}
		return nil
	})
	if err := registry.Use(plugin); err != nil {
		t.Fatal(err)
	}

	assertPluginConfigProvider(t, registry)
	assertPluginMiddleware(t, registry)
	assertPluginMetrics(t, registry)
}

func newTestPluginConfigProvider(_ context.Context, spec vela.ProviderSpec) (provider.ConfigProvider, error) {
	return testPluginConfigProvider{name: spec.Name}, nil
}

func markMiddleware(next http.Handler, _ vela.RuntimeMiddleware) http.Handler {
	return next
}

func newNoopPluginMetrics(bool, *slog.Logger) runtime.MetricsRecorder {
	return runtime.NewNoopMetrics()
}

func assertPluginConfigProvider(t *testing.T, registry *vela.Registry) {
	t.Helper()
	created, err := registry.CreateConfigProvider(context.Background(), vela.NewProviderSpec("memory").WithName("main"))
	if err != nil {
		t.Fatal(err)
	}
	if provider.ConfigProviderName(created, "") != "main" {
		t.Fatalf("provider name = %q, want main", provider.ConfigProviderName(created, ""))
	}
}

func assertPluginMiddleware(t *testing.T, registry *vela.Registry) {
	t.Helper()
	middlewares := registry.MiddlewareRegistry()
	if _, ok := middlewares.Factory("mark"); !ok {
		t.Fatal("middleware factory mark was not registered")
	}
}

func assertPluginMetrics(t *testing.T, registry *vela.Registry) {
	t.Helper()
	cfg := vela.DefaultConfig()
	if err := vela.WithMetricsFromRegistry(registry, "noop")(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Metrics == nil {
		t.Fatal("metrics factory was not applied")
	}
}

type testPluginConfigProvider struct {
	name string
}

func (p testPluginConfigProvider) Name() string {
	return p.name
}

func (testPluginConfigProvider) Load(context.Context) (*config.Config, error) {
	return config.Default(), nil
}

func (testPluginConfigProvider) Watch(context.Context, func(), func(error)) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

package vale_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
)

func TestRegistryUseRegistersCompileTimeExtension(t *testing.T) {
	t.Parallel()

	registry := vale.NewRegistry()
	if err := registry.Use(vale.ExtensionFunc(registerTestExtension)); err != nil {
		t.Fatal(err)
	}

	assertExtensionConfigProvider(t, registry)
	assertExtensionMiddleware(t, registry)
	assertExtensionMetrics(t, registry)
	assertExtensionCertificateStorage(t, registry)
	assertExtensionCluster(t, registry)
	assertExtensionObservability(t, registry)
}

func TestNewAcceptsRegisteredMiddlewareType(t *testing.T) {
	t.Parallel()

	registry := vale.NewRegistry()
	if err := registry.RegisterMiddleware("mark", markMiddleware); err != nil {
		t.Fatal(err)
	}
	cfg := vale.NewConfigBuilder().
		Entrypoint("web", "127.0.0.1:0").
		MiddlewareNamed("mark-request", vale.MiddlewareType("mark")).
		Service("api", "http://127.0.0.1:1").
		RouteTo("api-route", "web", "api", vale.RouteMiddlewares("mark-request")).
		Admin("127.0.0.1:0").
		Build()
	gateway, err := vale.New(
		vale.WithLogger(slog.New(slog.DiscardHandler)),
		vale.WithRegistry(registry),
		vale.WithStaticConfig(cfg),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := gateway.Stop(ctx); err != nil {
			t.Fatalf("stop gateway: %v", err)
		}
	})
}

func TestGatewayBuilderComposesExtensionAndOptions(t *testing.T) {
	t.Parallel()

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", "127.0.0.1:0").
		MiddlewareNamed("mark-request", vale.MiddlewareType("mark")).
		Service("api", "http://127.0.0.1:1").
		RouteTo("api-route", "web", "api", vale.RouteMiddlewares("mark-request")).
		Admin("127.0.0.1:0").
		Build()
	gateway, err := vale.NewGatewayBuilder(
		vale.GatewayExtensions(vale.ExtensionFunc(func(registry *vale.Registry) error {
			return registry.RegisterMiddleware("mark", markMiddleware)
		})),
		vale.GatewayOptions(
			vale.WithLogger(slog.New(slog.DiscardHandler)),
			vale.WithStaticConfig(cfg),
		),
	).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := gateway.Stop(ctx); err != nil {
			t.Fatalf("stop gateway: %v", err)
		}
	})
}

func registerTestExtension(registry *vale.Registry) error {
	if err := registry.RegisterConfigProvider("memory", newTestExtensionConfigProvider); err != nil {
		return fmt.Errorf("register config provider: %w", err)
	}
	if err := registry.RegisterMiddleware("mark", markMiddleware); err != nil {
		return fmt.Errorf("register middleware: %w", err)
	}
	if err := registry.RegisterMetricsFactory("noop", newNoopExtensionMetrics); err != nil {
		return fmt.Errorf("register metrics: %w", err)
	}
	if err := registry.RegisterCertificateStorage("memory", newMemoryCertificateStorage); err != nil {
		return fmt.Errorf("register certificate storage: %w", err)
	}
	if err := registry.RegisterClusterFactory("fake", newFakeCluster); err != nil {
		return fmt.Errorf("register cluster: %w", err)
	}
	if err := registry.RegisterObservabilityFactory("noop", newNoopObservability); err != nil {
		return fmt.Errorf("register observability: %w", err)
	}
	return nil
}

func newTestExtensionConfigProvider(_ context.Context, spec vale.ProviderSpec) (provider.ConfigProvider, error) {
	return testExtensionConfigProvider{name: spec.Name}, nil
}

func markMiddleware(next http.Handler, _ vale.RuntimeMiddleware) http.Handler {
	return next
}

func newNoopExtensionMetrics(bool, *slog.Logger) runtime.MetricsRecorder {
	return runtime.NewNoopMetrics()
}

func newMemoryCertificateStorage(context.Context) (certstore.Storage, error) {
	return certstore.NewProjection(), nil
}

func newNoopObservability(*slog.Logger) (observabilityx.Observability, error) {
	return observabilityx.Nop(), nil
}

func newFakeCluster(*slog.Logger) (vale.Cluster, error) {
	return fakeCluster{}, nil
}

func assertExtensionConfigProvider(t *testing.T, registry *vale.Registry) {
	t.Helper()
	created, err := registry.CreateConfigProvider(context.Background(), vale.NewProviderSpec("memory").WithName("main"))
	if err != nil {
		t.Fatal(err)
	}
	if provider.ConfigProviderName(created, "") != "main" {
		t.Fatalf("provider name = %q, want main", provider.ConfigProviderName(created, ""))
	}
}

func assertExtensionMiddleware(t *testing.T, registry *vale.Registry) {
	t.Helper()
	middlewares := registry.MiddlewareRegistry()
	if _, ok := middlewares.Factory("mark"); !ok {
		t.Fatal("middleware factory mark was not registered")
	}
}

func assertExtensionMetrics(t *testing.T, registry *vale.Registry) {
	t.Helper()
	cfg := vale.DefaultConfig()
	if err := vale.WithMetricsFromRegistry(registry, "noop")(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Metrics == nil {
		t.Fatal("metrics factory was not applied")
	}
}

func assertExtensionCertificateStorage(t *testing.T, registry *vale.Registry) {
	t.Helper()
	cfg := vale.DefaultConfig()
	if err := vale.WithCertificateStorageFromRegistry(context.Background(), registry, "memory")(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.CertificateStorage == nil {
		t.Fatal("certificate storage was not applied")
	}
	if !stringListContains(registry.CertificateStorageNames(), "memory") {
		t.Fatal("certificate storage name was not listed")
	}
}

func assertExtensionCluster(t *testing.T, registry *vale.Registry) {
	t.Helper()
	cfg := vale.DefaultConfig()
	if err := vale.WithClusterFromRegistry(registry, "fake")(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster == nil {
		t.Fatal("cluster factory was not applied")
	}
	if !stringListContains(registry.ClusterFactoryNames(), "fake") {
		t.Fatal("cluster factory name was not listed")
	}
}

func assertExtensionObservability(t *testing.T, registry *vale.Registry) {
	t.Helper()
	cfg := vale.DefaultConfig()
	cfg.Logger = slog.New(slog.DiscardHandler)
	if err := vale.WithObservabilityFromRegistry(registry, "noop")(&cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Observability == nil {
		t.Fatal("observability was not applied")
	}
	if !stringListContains(registry.ObservabilityFactoryNames(), "noop") {
		t.Fatal("observability factory name was not listed")
	}
}

func stringListContains(values *list.List[string], expected string) bool {
	if values == nil {
		return false
	}
	found := false
	values.Range(func(_ int, value string) bool {
		found = value == expected
		return !found
	})
	return found
}

type testExtensionConfigProvider struct {
	name string
}

func (p testExtensionConfigProvider) Name() string {
	return p.name
}

func (testExtensionConfigProvider) Load(context.Context) (*config.Config, error) {
	return config.Default(), nil
}

func (testExtensionConfigProvider) Watch(context.Context, func(), func(error)) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

type fakeCluster struct{}

func (fakeCluster) IsLeader() bool {
	return true
}

func (fakeCluster) Apply([]byte, time.Duration) error {
	return nil
}

func (fakeCluster) Status() *mapping.Map[string, any] {
	return mapping.NewMap[string, any]()
}

func (fakeCluster) Peers() (*list.List[*vale.ClusterPeer], error) {
	return list.NewList[*vale.ClusterPeer](), nil
}

func (fakeCluster) AddVoter(string, string, time.Duration) error {
	return nil
}

func (fakeCluster) RemoveServer(string, time.Duration) error {
	return nil
}

func (fakeCluster) Shutdown() error {
	return nil
}

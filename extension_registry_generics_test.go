package vale_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
)

func TestRegistryAcceptsConcreteFactories(t *testing.T) {
	t.Parallel()

	newConcreteRegistry(t)
}

func TestRegistryCreatesConcreteConfigProvider(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	created, err := registry.CreateConfigProviderAs[*testExtensionConfigProvider](
		context.Background(),
		vale.NewProviderSpec("config").WithName("main"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.name != "main" {
		t.Fatalf("config provider name = %q, want main", created.name)
	}
}

func TestRegistryCreatesConcreteSnapshotProvider(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	created, err := registry.CreateSnapshotProviderAs[*testSnapshotProvider](
		context.Background(),
		vale.NewProviderSpec("snapshot"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if created == nil {
		t.Fatal("snapshot provider is nil")
	}
}

func TestRegistryCreatesConcreteCertificateStorage(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	created, err := registry.CreateCertificateStorageAs[*certstore.Projection](
		context.Background(),
		"certificate",
	)
	if err != nil {
		t.Fatal(err)
	}
	if created == nil {
		t.Fatal("certificate storage is nil")
	}
}

func TestRegistryCreatesConcreteObservability(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	created, err := registry.CreateObservabilityAs[*testObservability](
		"observability",
		slog.New(slog.DiscardHandler),
	)
	if err != nil {
		t.Fatal(err)
	}
	if created == nil {
		t.Fatal("observability is nil")
	}
}

func TestRegistryConfigProviderTypeMismatch(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	_, err := registry.CreateConfigProviderAs[testExtensionConfigProvider](
		context.Background(),
		vale.NewProviderSpec("config"),
	)
	assertConcreteTypeMismatch(
		t,
		err,
		fmt.Sprintf("%T", testExtensionConfigProvider{}),
		fmt.Sprintf("%T", (*testExtensionConfigProvider)(nil)),
	)
}

func TestRegistrySnapshotProviderTypeMismatch(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	_, err := registry.CreateSnapshotProviderAs[testSnapshotProviderValue](
		context.Background(),
		vale.NewProviderSpec("snapshot"),
	)
	assertConcreteTypeMismatch(
		t,
		err,
		fmt.Sprintf("%T", testSnapshotProviderValue{}),
		fmt.Sprintf("%T", (*testSnapshotProvider)(nil)),
	)
}

func TestRegistryCertificateStorageTypeMismatch(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	_, err := registry.CreateCertificateStorageAs[*certstore.RaftStorage](
		context.Background(),
		"certificate",
	)
	assertConcreteTypeMismatch(
		t,
		err,
		fmt.Sprintf("%T", (*certstore.RaftStorage)(nil)),
		fmt.Sprintf("%T", (*certstore.Projection)(nil)),
	)
}

func TestRegistryObservabilityTypeMismatch(t *testing.T) {
	t.Parallel()

	registry := newConcreteRegistry(t)
	_, err := registry.CreateObservabilityAs[testObservability](
		"observability",
		slog.New(slog.DiscardHandler),
	)
	assertConcreteTypeMismatch(
		t,
		err,
		fmt.Sprintf("%T", testObservability{}),
		fmt.Sprintf("%T", (*testObservability)(nil)),
	)
}

func newConcreteRegistry(t *testing.T) *vale.Registry {
	t.Helper()
	registry := vale.NewRegistry()
	if err := registry.RegisterConfigProvider("config", newConcreteConfigProvider); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterSnapshotProvider("snapshot", newConcreteSnapshotProvider); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterMiddleware("middleware", newConcreteMiddleware); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterMetricsFactory("metrics", newConcreteMetrics); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterCertificateStorage("certificate", newConcreteCertificateStorage); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterClusterFactory("cluster", newConcreteCluster); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterObservabilityFactory("observability", newConcreteObservability); err != nil {
		t.Fatal(err)
	}
	return registry
}

func assertConcreteTypeMismatch(t *testing.T, err error, expectedType, actualType string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected concrete type mismatch")
	}
	if !strings.Contains(err.Error(), expectedType) {
		t.Fatalf("error %q does not contain expected type %q", err, expectedType)
	}
	if !strings.Contains(err.Error(), actualType) {
		t.Fatalf("error %q does not contain actual type %q", err, actualType)
	}
}

func newConcreteConfigProvider(_ context.Context, spec vale.ProviderSpec) (*testExtensionConfigProvider, error) {
	return &testExtensionConfigProvider{name: spec.Name}, nil
}

func newConcreteSnapshotProvider(context.Context, vale.ProviderSpec) (*testSnapshotProvider, error) {
	return &testSnapshotProvider{}, nil
}

func newConcreteMiddleware(next http.Handler, _ vale.RuntimeMiddleware) http.HandlerFunc {
	return next.ServeHTTP
}

func newConcreteMetrics(bool, *slog.Logger) testMetricsRecorder {
	return testMetricsRecorder{}
}

func newConcreteCertificateStorage(context.Context) (*certstore.Projection, error) {
	return certstore.NewProjection(), nil
}

func newConcreteCluster(*slog.Logger) (fakeCluster, error) {
	return fakeCluster{}, nil
}

func newConcreteObservability(*slog.Logger) (*testObservability, error) {
	return &testObservability{Observability: observabilityx.Nop()}, nil
}

type testSnapshotProvider struct{}

func (*testSnapshotProvider) Load(context.Context) (*runtime.CompiledSnapshot, error) {
	return runtime.NewSnapshot(), nil
}

func (*testSnapshotProvider) Watch(
	context.Context,
	func(*runtime.CompiledSnapshot),
	func(error),
) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

type testSnapshotProviderValue struct{}

func (testSnapshotProviderValue) Load(context.Context) (*runtime.CompiledSnapshot, error) {
	return runtime.NewSnapshot(), nil
}

func (testSnapshotProviderValue) Watch(
	context.Context,
	func(*runtime.CompiledSnapshot),
	func(error),
) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

type testMetricsRecorder struct{}

func (testMetricsRecorder) Observe(*runtime.CompiledRoute, *runtime.EndpointRuntime, int, time.Duration) {
}

func (testMetricsRecorder) Handler() http.Handler {
	return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
}

type testObservability struct {
	observabilityx.Observability
}

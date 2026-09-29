package vale_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/arcgolabs/vale"
	"github.com/arcgolabs/vale/certstore"
)

func TestRegistryRejectsTypedNilCertificateStorage(t *testing.T) {
	t.Parallel()

	registry := vale.NewRegistry()
	if err := registry.RegisterCertificateStorage("nil", func(context.Context) (*certstore.Projection, error) {
		return nilPointer[certstore.Projection](), nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.CreateCertificateStorage(context.Background(), "nil")
	assertTypedNilFactoryError(t, err)
}

func TestRegistryRejectsTypedNilObservability(t *testing.T) {
	t.Parallel()

	registry := vale.NewRegistry()
	if err := registry.RegisterObservabilityFactory("nil", func(*slog.Logger) (*testObservability, error) {
		return nilPointer[testObservability](), nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.CreateObservability("nil", slog.New(slog.DiscardHandler))
	assertTypedNilFactoryError(t, err)
}

func TestRegistryRejectsTypedNilCluster(t *testing.T) {
	t.Parallel()

	registry := vale.NewRegistry()
	if err := registry.RegisterClusterFactory("nil", func(*slog.Logger) (*fakeCluster, error) {
		return nilPointer[fakeCluster](), nil
	}); err != nil {
		t.Fatal(err)
	}
	factory, ok := registry.ClusterFactory("nil")
	if !ok {
		t.Fatal("cluster factory is not registered")
	}
	_, err := factory(slog.New(slog.DiscardHandler))
	assertTypedNilFactoryError(t, err)
}

func TestRegistryNormalizesTypedNilMetrics(t *testing.T) {
	t.Parallel()

	registry := vale.NewRegistry()
	if err := registry.RegisterMetricsFactory("nil", func(bool, *slog.Logger) *testMetricsRecorder {
		return nilPointer[testMetricsRecorder]()
	}); err != nil {
		t.Fatal(err)
	}
	factory, ok := registry.MetricsFactory("nil")
	if !ok {
		t.Fatal("metrics factory is not registered")
	}
	if metrics := factory(true, slog.New(slog.DiscardHandler)); metrics != nil {
		t.Fatalf("metrics = %#v, want nil", metrics)
	}
}

func assertTypedNilFactoryError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "returned nil") {
		t.Fatalf("factory error = %v, want typed-nil factory error", err)
	}
}

func nilPointer[T any]() *T {
	return nil
}

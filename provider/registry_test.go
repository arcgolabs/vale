package provider_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestConfigProviderRegistryCreatesProvider(t *testing.T) {
	t.Parallel()

	registry := provider.NewConfigProviderRegistry()
	if err := registry.Register("Memory", func(_ context.Context, spec provider.ProviderSpec) (testConfigProvider, error) {
		return testConfigProvider{name: spec.Name}, nil
	}); err != nil {
		t.Fatal(err)
	}

	created, err := registry.CreateAs[testConfigProvider](
		context.Background(),
		provider.NewProviderSpec(" memory ").WithName("main"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name() != "main" {
		t.Fatalf("provider name = %q, want main", created.Name())
	}
	if registry.Names().Values()[0] != "memory" {
		t.Fatalf("names = %v, want [memory]", registry.Names().Values())
	}
}

func TestSnapshotProviderRegistryRejectsUnknownType(t *testing.T) {
	t.Parallel()

	_, err := provider.NewSnapshotProviderRegistry().Create(context.Background(), provider.NewProviderSpec("missing"))
	if err == nil {
		t.Fatal("Create returned nil error")
	}
}

func TestSnapshotProviderRegistryCreatesConcreteProvider(t *testing.T) {
	t.Parallel()

	registry := provider.NewSnapshotProviderRegistry()
	if err := registry.Register("memory", func(context.Context, provider.ProviderSpec) (testSnapshotProvider, error) {
		return testSnapshotProvider{}, nil
	}); err != nil {
		t.Fatal(err)
	}

	created, err := registry.CreateAs[testSnapshotProvider](
		context.Background(),
		provider.NewProviderSpec("memory"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := created.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConfigProviderRegistryCreateAsRejectsUnexpectedConcreteType(t *testing.T) {
	t.Parallel()

	registry := provider.NewConfigProviderRegistry()
	if err := registry.Register("memory", func(context.Context, provider.ProviderSpec) (testConfigProvider, error) {
		return testConfigProvider{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.CreateAs[*testConfigProvider](
		context.Background(),
		provider.NewProviderSpec("memory"),
	); err == nil {
		t.Fatal("CreateAs returned nil error for an unexpected concrete type")
	}
}

func TestConfigProviderRegistryRejectsTypedNilFactoryResult(t *testing.T) {
	t.Parallel()

	registry := provider.NewConfigProviderRegistry()
	if err := registry.Register("nil", func(context.Context, provider.ProviderSpec) (*testConfigProvider, error) {
		return nilProviderPointer[testConfigProvider](), nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Create(context.Background(), provider.NewProviderSpec("nil"))
	if err == nil || !strings.Contains(err.Error(), "returned nil") {
		t.Fatalf("Create error = %v, want typed-nil factory error", err)
	}
}

func TestSnapshotProviderRegistryRejectsTypedNilFactoryResult(t *testing.T) {
	t.Parallel()

	registry := provider.NewSnapshotProviderRegistry()
	if err := registry.Register("nil", func(context.Context, provider.ProviderSpec) (*testSnapshotProvider, error) {
		return nilProviderPointer[testSnapshotProvider](), nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Create(context.Background(), provider.NewProviderSpec("nil"))
	if err == nil || !strings.Contains(err.Error(), "returned nil") {
		t.Fatalf("Create error = %v, want typed-nil factory error", err)
	}
}

func nilProviderPointer[T any]() *T {
	return nil
}

type testConfigProvider struct {
	name string
}

func (p testConfigProvider) Name() string {
	return p.name
}

func (testConfigProvider) Load(context.Context) (*config.Config, error) {
	return config.Default(), nil
}

func (testConfigProvider) Watch(context.Context, func(), func(error)) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

type testSnapshotProvider struct{}

func (testSnapshotProvider) Load(context.Context) (*valeruntime.CompiledSnapshot, error) {
	return valeruntime.NewSnapshot(), nil
}

func (testSnapshotProvider) Watch(
	context.Context,
	func(*valeruntime.CompiledSnapshot),
	func(error),
) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

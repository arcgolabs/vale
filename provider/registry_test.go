package provider_test

import (
	"context"
	"io"
	"testing"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
)

func TestConfigProviderRegistryCreatesProvider(t *testing.T) {
	t.Parallel()

	registry := provider.NewConfigProviderRegistry()
	if err := registry.Register("Memory", func(_ context.Context, spec provider.ProviderSpec) (provider.ConfigProvider, error) {
		return testConfigProvider{name: spec.Name}, nil
	}); err != nil {
		t.Fatal(err)
	}

	created, err := registry.Create(context.Background(), provider.NewProviderSpec(" memory ").WithName("main"))
	if err != nil {
		t.Fatal(err)
	}
	if provider.ConfigProviderName(created, "") != "main" {
		t.Fatalf("provider name = %q, want main", provider.ConfigProviderName(created, ""))
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

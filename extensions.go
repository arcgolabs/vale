package vale

import (
	"context"
	"log/slog"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/gateway"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
	"github.com/samber/oops"
)

type (
	Extension     interface{ Register(*Registry) error }
	ExtensionFunc func(*Registry) error
	Registry      struct {
		configProviders     *provider.ConfigProviderRegistry
		snapshotProviders   *provider.SnapshotProviderRegistry
		middleware          *runtime.MiddlewareRegistry
		metrics             *mapping.Map[string, gateway.MetricsFactory]
		certificateStorages *mapping.Map[string, CertificateStorageFactory]
		clusters            *mapping.Map[string, gateway.ClusterFactory]
		observability       *mapping.Map[string, ObservabilityFactory]
	}
	ProviderSpec              = provider.ProviderSpec
	ConfigProviderFactory     = provider.ConfigProviderFactory
	SnapshotProviderFactory   = provider.SnapshotProviderFactory
	MetricsFactory            = gateway.MetricsFactory
	ClusterFactory            = gateway.ClusterFactory
	CertificateStorage        = certstore.Storage
	CertificateStorageFactory func(context.Context) (certstore.Storage, error)
	ObservabilityFactory      func(*slog.Logger) (observabilityx.Observability, error)
)

func (fn ExtensionFunc) Register(registry *Registry) error {
	if fn == nil {
		return oops.In("vale").New("extension function cannot be nil")
	}
	return fn(registry)
}

func NewRegistry() *Registry {
	return &Registry{
		configProviders:     provider.NewConfigProviderRegistry(),
		snapshotProviders:   provider.NewSnapshotProviderRegistry(),
		middleware:          runtime.DefaultMiddlewareRegistry(),
		metrics:             mapping.NewMap[string, gateway.MetricsFactory](),
		certificateStorages: mapping.NewMap[string, CertificateStorageFactory](),
		clusters:            mapping.NewMap[string, gateway.ClusterFactory](),
		observability:       mapping.NewMap[string, ObservabilityFactory](),
	}
}

func (r *Registry) Use(extensions ...Extension) error {
	if r == nil {
		return oops.In("vale").New("registry cannot be nil")
	}
	for _, extension := range extensions {
		if extension == nil {
			continue
		}
		if err := extension.Register(r); err != nil {
			return oops.In("vale").Wrapf(err, "register extension")
		}
	}
	return nil
}

func (r *Registry) RegisterConfigProvider(providerType string, factory ConfigProviderFactory) error {
	r.ensureInit()
	if err := r.configProviders.Register(providerType, factory); err != nil {
		return oops.In("vale").With("type", providerType).Wrapf(err, "register config provider factory")
	}
	return nil
}

func (r *Registry) CreateConfigProvider(ctx context.Context, spec ProviderSpec) (provider.ConfigProvider, error) {
	if r == nil {
		return nil, oops.In("vale").New("registry cannot be nil")
	}
	r.ensureInit()
	configProvider, err := r.configProviders.Create(ctx, spec)
	if err != nil {
		return nil, oops.In("vale").With("type", spec.Type, "name", spec.Name).Wrapf(err, "create config provider")
	}
	return configProvider, nil
}

func (r *Registry) ConfigProviderTypes() *collectionlist.List[string] {
	if r == nil || r.configProviders == nil {
		return collectionlist.NewList[string]()
	}
	return r.configProviders.Names()
}

func (r *Registry) RegisterSnapshotProvider(providerType string, factory SnapshotProviderFactory) error {
	r.ensureInit()
	if err := r.snapshotProviders.Register(providerType, factory); err != nil {
		return oops.In("vale").With("type", providerType).Wrapf(err, "register snapshot provider factory")
	}
	return nil
}

func (r *Registry) CreateSnapshotProvider(ctx context.Context, spec ProviderSpec) (provider.SnapshotProvider, error) {
	if r == nil {
		return nil, oops.In("vale").New("registry cannot be nil")
	}
	r.ensureInit()
	snapshotProvider, err := r.snapshotProviders.Create(ctx, spec)
	if err != nil {
		return nil, oops.In("vale").With("type", spec.Type, "name", spec.Name).Wrapf(err, "create snapshot provider")
	}
	return snapshotProvider, nil
}

func (r *Registry) SnapshotProviderTypes() *collectionlist.List[string] {
	if r == nil || r.snapshotProviders == nil {
		return collectionlist.NewList[string]()
	}
	return r.snapshotProviders.Names()
}

func (r *Registry) RegisterMiddleware(middlewareType string, factory MiddlewareFactory) error {
	r.ensureInit()
	if err := r.middleware.Register(middlewareType, factory); err != nil {
		return oops.In("vale").With("type", middlewareType).Wrapf(err, "register middleware factory")
	}
	return nil
}

func (r *Registry) MiddlewareRegistry() *runtime.MiddlewareRegistry {
	if r == nil || r.middleware == nil {
		return runtime.DefaultMiddlewareRegistry()
	}
	return r.middleware.Clone()
}

func (r *Registry) RegisterMetricsFactory(name string, factory MetricsFactory) error {
	name = normalizeRegistryName(name)
	if name == "" {
		return oops.In("vale").New("metrics factory name cannot be empty")
	}
	if factory == nil {
		return oops.In("vale").With("name", name).New("metrics factory cannot be nil")
	}
	r.ensureInit()
	r.metrics.Set(name, factory)
	return nil
}

func (r *Registry) MetricsFactory(name string) (MetricsFactory, bool) {
	if r == nil || r.metrics == nil {
		return nil, false
	}
	return r.metrics.Get(normalizeRegistryName(name))
}

func (r *Registry) MetricsFactoryNames() *collectionlist.List[string] {
	if r == nil || r.metrics == nil {
		return collectionlist.NewList[string]()
	}
	return sortedRegistryNames(r.metrics)
}

func WithRegistry(registry *Registry) Option {
	return func(cfg *Config) error {
		if registry == nil {
			return oops.In("vale").New("registry cannot be nil")
		}
		return gateway.WithMiddlewareRegistry(registry.MiddlewareRegistry())(cfg)
	}
}

func WithExtensions(extensions ...Extension) Option {
	return func(cfg *Config) error {
		registry := NewRegistry()
		if err := registry.Use(extensions...); err != nil {
			return err
		}
		return WithRegistry(registry)(cfg)
	}
}

func WithMetricsFromRegistry(registry *Registry, name string) Option {
	return func(cfg *Config) error {
		factory, ok := registry.MetricsFactory(name)
		if !ok {
			return oops.In("vale").With("name", name).New("metrics factory is not registered")
		}
		return gateway.WithMetricsFactory(factory)(cfg)
	}
}

func NewProviderSpec(providerType string) ProviderSpec {
	return provider.NewProviderSpec(providerType)
}

func (r *Registry) ensureInit() {
	if r.configProviders == nil {
		r.configProviders = provider.NewConfigProviderRegistry()
	}
	if r.snapshotProviders == nil {
		r.snapshotProviders = provider.NewSnapshotProviderRegistry()
	}
	if r.middleware == nil {
		r.middleware = runtime.DefaultMiddlewareRegistry()
	}
	if r.metrics == nil {
		r.metrics = mapping.NewMap[string, gateway.MetricsFactory]()
	}
	if r.certificateStorages == nil {
		r.certificateStorages = mapping.NewMap[string, CertificateStorageFactory]()
	}
	if r.clusters == nil {
		r.clusters = mapping.NewMap[string, gateway.ClusterFactory]()
	}
	if r.observability == nil {
		r.observability = mapping.NewMap[string, ObservabilityFactory]()
	}
}

func normalizeRegistryName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func sortedRegistryNames[T any](values *mapping.Map[string, T]) *collectionlist.List[string] {
	if values == nil {
		return collectionlist.NewList[string]()
	}
	return collectionlist.NewList(values.Keys()...).Sort(strings.Compare)
}

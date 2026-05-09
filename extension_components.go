package vale

import (
	"context"
	"log/slog"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/gateway"
	"github.com/samber/oops"
)

func (r *Registry) RegisterCertificateStorage(name string, factory CertificateStorageFactory) error {
	name = normalizeRegistryName(name)
	if name == "" {
		return oops.In("vale").New("certificate storage name cannot be empty")
	}
	if factory == nil {
		return oops.In("vale").With("name", name).New("certificate storage factory cannot be nil")
	}
	r.ensureInit()
	r.certificateStorages.Set(name, factory)
	return nil
}

func (r *Registry) CreateCertificateStorage(ctx context.Context, name string) (certstore.Storage, error) {
	if r == nil {
		return nil, oops.In("vale").New("registry cannot be nil")
	}
	r.ensureInit()
	name = normalizeRegistryName(name)
	factory, ok := r.certificateStorages.Get(name)
	if !ok {
		return nil, oops.In("vale").With("name", name).New("certificate storage factory is not registered")
	}
	storage, err := factory(ctx)
	if err != nil {
		return nil, oops.In("vale").With("name", name).Wrapf(err, "create certificate storage")
	}
	if storage == nil {
		return nil, oops.In("vale").With("name", name).New("certificate storage factory returned nil")
	}
	return storage, nil
}

func (r *Registry) CertificateStorageNames() *collectionlist.List[string] {
	if r == nil || r.certificateStorages == nil {
		return collectionlist.NewList[string]()
	}
	return sortedRegistryNames(r.certificateStorages)
}

func (r *Registry) RegisterClusterFactory(name string, factory ClusterFactory) error {
	name = normalizeRegistryName(name)
	if name == "" {
		return oops.In("vale").New("cluster factory name cannot be empty")
	}
	if factory == nil {
		return oops.In("vale").With("name", name).New("cluster factory cannot be nil")
	}
	r.ensureInit()
	r.clusters.Set(name, factory)
	return nil
}

func (r *Registry) ClusterFactory(name string) (ClusterFactory, bool) {
	if r == nil || r.clusters == nil {
		return nil, false
	}
	return r.clusters.Get(normalizeRegistryName(name))
}

func (r *Registry) ClusterFactoryNames() *collectionlist.List[string] {
	if r == nil || r.clusters == nil {
		return collectionlist.NewList[string]()
	}
	return sortedRegistryNames(r.clusters)
}

func (r *Registry) RegisterObservabilityFactory(name string, factory ObservabilityFactory) error {
	name = normalizeRegistryName(name)
	if name == "" {
		return oops.In("vale").New("observability factory name cannot be empty")
	}
	if factory == nil {
		return oops.In("vale").With("name", name).New("observability factory cannot be nil")
	}
	r.ensureInit()
	r.observability.Set(name, factory)
	return nil
}

func (r *Registry) CreateObservability(name string, logger *slog.Logger) (observabilityx.Observability, error) {
	if r == nil {
		return nil, oops.In("vale").New("registry cannot be nil")
	}
	r.ensureInit()
	name = normalizeRegistryName(name)
	factory, ok := r.observability.Get(name)
	if !ok {
		return nil, oops.In("vale").With("name", name).New("observability factory is not registered")
	}
	if logger == nil {
		logger = slog.Default()
	}
	obs, err := factory(logger)
	if err != nil {
		return nil, oops.In("vale").With("name", name).Wrapf(err, "create observability")
	}
	if obs == nil {
		return nil, oops.In("vale").With("name", name).New("observability factory returned nil")
	}
	return obs, nil
}

func (r *Registry) ObservabilityFactoryNames() *collectionlist.List[string] {
	if r == nil || r.observability == nil {
		return collectionlist.NewList[string]()
	}
	return sortedRegistryNames(r.observability)
}

func WithCertificateStorageFromRegistry(ctx context.Context, registry *Registry, name string) Option {
	return func(cfg *Config) error {
		storage, err := registry.CreateCertificateStorage(ctx, name)
		if err != nil {
			return err
		}
		return gateway.WithCertificateStorage(storage)(cfg)
	}
}

func WithClusterFromRegistry(registry *Registry, name string) Option {
	return func(cfg *Config) error {
		factory, ok := registry.ClusterFactory(name)
		if !ok {
			return oops.In("vale").With("name", name).New("cluster factory is not registered")
		}
		return gateway.WithClusterFactory(factory)(cfg)
	}
}

func WithObservabilityFromRegistry(registry *Registry, name string) Option {
	return func(cfg *Config) error {
		obs, err := registry.CreateObservability(name, cfg.Logger)
		if err != nil {
			return err
		}
		return gateway.WithObservability(obs)(cfg)
	}
}

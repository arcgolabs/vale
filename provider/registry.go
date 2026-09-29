package provider

import (
	"context"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vale/internal/genericx"
	"github.com/samber/oops"
)

type ProviderSpec struct {
	Name     string
	Type     string
	Settings *mapping.Map[string, string]
}

type ConfigProviderFactory func(context.Context, ProviderSpec) (ConfigProvider, error)
type SnapshotProviderFactory func(context.Context, ProviderSpec) (SnapshotProvider, error)

type ConfigProviderRegistry struct {
	factories *mapping.Map[string, ConfigProviderFactory]
}

type SnapshotProviderRegistry struct {
	factories *mapping.Map[string, SnapshotProviderFactory]
}

func NewProviderSpec(providerType string) ProviderSpec {
	return ProviderSpec{
		Type:     normalizeProviderType(providerType),
		Settings: mapping.NewMap[string, string](),
	}
}

func (s ProviderSpec) WithName(name string) ProviderSpec {
	s.Name = strings.TrimSpace(name)
	return s
}

func (s ProviderSpec) WithSetting(key, value string) ProviderSpec {
	s.Settings = cloneSpecSettings(s.Settings)
	if key = strings.TrimSpace(key); key != "" {
		s.Settings.Set(key, strings.TrimSpace(value))
	}
	return s
}

func (s ProviderSpec) Setting(key string) (string, bool) {
	if s.Settings == nil {
		return "", false
	}
	return s.Settings.Get(strings.TrimSpace(key))
}

func NewConfigProviderRegistry() *ConfigProviderRegistry {
	return &ConfigProviderRegistry{
		factories: mapping.NewMap[string, ConfigProviderFactory](),
	}
}

func (r *ConfigProviderRegistry) Register[P ConfigProvider](
	providerType string,
	factory func(context.Context, ProviderSpec) (P, error),
) error {
	if genericx.IsNil(factory) {
		return oops.In("provider").With("type", providerType).New("provider factory cannot be nil")
	}
	return r.RegisterFactory(providerType, func(ctx context.Context, spec ProviderSpec) (ConfigProvider, error) {
		return factory(ctx, spec)
	})
}

func (r *ConfigProviderRegistry) RegisterFactory(providerType string, factory ConfigProviderFactory) error {
	providerType, erased, err := prepareProviderFactory(
		providerType,
		"config provider",
		factory,
		func(created ConfigProvider) ConfigProvider { return created },
	)
	if err != nil {
		return err
	}
	r.ensureInit()
	r.factories.Set(providerType, erased)
	return nil
}

func (r *ConfigProviderRegistry) Factory(providerType string) (ConfigProviderFactory, bool) {
	if r == nil || r.factories == nil {
		return nil, false
	}
	return r.factories.Get(normalizeProviderType(providerType))
}

func (r *ConfigProviderRegistry) Create(ctx context.Context, spec ProviderSpec) (ConfigProvider, error) {
	factory, ok := r.Factory(spec.Type)
	if !ok {
		return nil, oops.In("provider").With("type", spec.Type).New("config provider factory is not registered")
	}
	provider, err := factory(ctx, spec)
	if err != nil {
		return nil, oops.In("provider").With("type", spec.Type, "name", spec.Name).Wrapf(err, "create config provider")
	}
	if provider == nil {
		return nil, oops.In("provider").With("type", spec.Type, "name", spec.Name).New("config provider factory returned nil")
	}
	return provider, nil
}

func (r *ConfigProviderRegistry) CreateAs[P ConfigProvider](ctx context.Context, spec ProviderSpec) (P, error) {
	var zero P
	created, err := r.Create(ctx, spec)
	if err != nil {
		return zero, err
	}
	typed, ok := created.(P)
	if !ok {
		return zero, oops.
			In("provider").
			With("type", spec.Type, "name", spec.Name, "actual_type", fmt.Sprintf("%T", created)).
			New("config provider has unexpected type")
	}
	return typed, nil
}

func (r *ConfigProviderRegistry) Names() *collectionlist.List[string] {
	if r == nil || r.factories == nil {
		return collectionlist.NewList[string]()
	}
	return SortedStrings(collectionlist.NewList(r.factories.Keys()...))
}

func (r *ConfigProviderRegistry) Clone() *ConfigProviderRegistry {
	if r == nil || r.factories == nil {
		return NewConfigProviderRegistry()
	}
	return &ConfigProviderRegistry{factories: r.factories.Clone()}
}

func NewSnapshotProviderRegistry() *SnapshotProviderRegistry {
	return &SnapshotProviderRegistry{
		factories: mapping.NewMap[string, SnapshotProviderFactory](),
	}
}

func (r *SnapshotProviderRegistry) Register[P SnapshotProvider](
	providerType string,
	factory func(context.Context, ProviderSpec) (P, error),
) error {
	if genericx.IsNil(factory) {
		return oops.In("provider").With("type", providerType).New("provider factory cannot be nil")
	}
	return r.RegisterFactory(providerType, func(ctx context.Context, spec ProviderSpec) (SnapshotProvider, error) {
		return factory(ctx, spec)
	})
}

func (r *SnapshotProviderRegistry) RegisterFactory(providerType string, factory SnapshotProviderFactory) error {
	providerType, erased, err := prepareProviderFactory(
		providerType,
		"snapshot provider",
		factory,
		func(created SnapshotProvider) SnapshotProvider { return created },
	)
	if err != nil {
		return err
	}
	r.ensureInit()
	r.factories.Set(providerType, erased)
	return nil
}

func (r *SnapshotProviderRegistry) Factory(providerType string) (SnapshotProviderFactory, bool) {
	if r == nil || r.factories == nil {
		return nil, false
	}
	return r.factories.Get(normalizeProviderType(providerType))
}

func (r *SnapshotProviderRegistry) Create(ctx context.Context, spec ProviderSpec) (SnapshotProvider, error) {
	factory, ok := r.Factory(spec.Type)
	if !ok {
		return nil, oops.In("provider").With("type", spec.Type).New("snapshot provider factory is not registered")
	}
	provider, err := factory(ctx, spec)
	if err != nil {
		return nil, oops.In("provider").With("type", spec.Type, "name", spec.Name).Wrapf(err, "create snapshot provider")
	}
	if provider == nil {
		return nil, oops.In("provider").With("type", spec.Type, "name", spec.Name).New("snapshot provider factory returned nil")
	}
	return provider, nil
}

func (r *SnapshotProviderRegistry) CreateAs[P SnapshotProvider](ctx context.Context, spec ProviderSpec) (P, error) {
	var zero P
	created, err := r.Create(ctx, spec)
	if err != nil {
		return zero, err
	}
	typed, ok := created.(P)
	if !ok {
		return zero, oops.
			In("provider").
			With("type", spec.Type, "name", spec.Name, "actual_type", fmt.Sprintf("%T", created)).
			New("snapshot provider has unexpected type")
	}
	return typed, nil
}

func (r *SnapshotProviderRegistry) Names() *collectionlist.List[string] {
	if r == nil || r.factories == nil {
		return collectionlist.NewList[string]()
	}
	return SortedStrings(collectionlist.NewList(r.factories.Keys()...))
}

func (r *SnapshotProviderRegistry) Clone() *SnapshotProviderRegistry {
	if r == nil || r.factories == nil {
		return NewSnapshotProviderRegistry()
	}
	return &SnapshotProviderRegistry{factories: r.factories.Clone()}
}

func (r *ConfigProviderRegistry) ensureInit() {
	if r.factories == nil {
		r.factories = mapping.NewMap[string, ConfigProviderFactory]()
	}
}

func (r *SnapshotProviderRegistry) ensureInit() {
	if r.factories == nil {
		r.factories = mapping.NewMap[string, SnapshotProviderFactory]()
	}
}

func normalizeProviderType(providerType string) string {
	return strings.ToLower(strings.TrimSpace(providerType))
}

func cloneSpecSettings(settings *mapping.Map[string, string]) *mapping.Map[string, string] {
	if settings == nil {
		return mapping.NewMap[string, string]()
	}
	return settings.Clone()
}

func prepareProviderFactory[P, I any](
	providerType, resultName string,
	factory func(context.Context, ProviderSpec) (P, error),
	erase func(P) I,
) (string, func(context.Context, ProviderSpec) (I, error), error) {
	providerType = normalizeProviderType(providerType)
	if providerType == "" {
		return "", nil, oops.In("provider").New("provider type cannot be empty")
	}
	if genericx.IsNil(factory) {
		return "", nil, oops.In("provider").With("type", providerType).New("provider factory cannot be nil")
	}
	erased := func(ctx context.Context, spec ProviderSpec) (I, error) {
		created, err := factory(ctx, spec)
		if err != nil {
			var zero I
			return zero, err
		}
		if genericx.IsNil(created) {
			var zero I
			return zero, oops.
				In("provider").
				With("type", providerType, "name", spec.Name).
				New(resultName + " factory returned nil")
		}
		return erase(created), nil
	}
	return providerType, erased, nil
}

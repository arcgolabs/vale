package provider

import (
	"context"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
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

func (r *ConfigProviderRegistry) Register(providerType string, factory ConfigProviderFactory) error {
	providerType = normalizeProviderType(providerType)
	if err := validateConfigProviderFactory(providerType, factory); err != nil {
		return err
	}
	r.ensureInit()
	r.factories.Set(providerType, factory)
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

func (r *ConfigProviderRegistry) Names() *collectionlist.List[string] {
	if r == nil || r.factories == nil {
		return collectionlist.NewList[string]()
	}
	return collectionlist.NewList(SortedStrings(r.factories.Keys())...)
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

func (r *SnapshotProviderRegistry) Register(providerType string, factory SnapshotProviderFactory) error {
	providerType = normalizeProviderType(providerType)
	if err := validateSnapshotProviderFactory(providerType, factory); err != nil {
		return err
	}
	r.ensureInit()
	r.factories.Set(providerType, factory)
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

func (r *SnapshotProviderRegistry) Names() *collectionlist.List[string] {
	if r == nil || r.factories == nil {
		return collectionlist.NewList[string]()
	}
	return collectionlist.NewList(SortedStrings(r.factories.Keys())...)
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

func validateConfigProviderFactory(providerType string, factory ConfigProviderFactory) error {
	if providerType == "" {
		return oops.In("provider").New("provider type cannot be empty")
	}
	if factory == nil {
		return oops.In("provider").With("type", providerType).New("provider factory cannot be nil")
	}
	return nil
}

func validateSnapshotProviderFactory(providerType string, factory SnapshotProviderFactory) error {
	if providerType == "" {
		return oops.In("provider").New("provider type cannot be empty")
	}
	if factory == nil {
		return oops.In("provider").With("type", providerType).New("provider factory cannot be nil")
	}
	return nil
}

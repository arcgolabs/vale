package vale

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

type (
	// GatewayComponent is the library-level composition unit used by embedded
	// applications and by the standalone valed binary assembly.
	GatewayComponent interface {
		ConfigureGateway(*GatewayBuilder) error
	}

	GatewayComponentFunc func(*GatewayBuilder) error

	GatewayBuilder struct {
		options            *collectionlist.List[Option]
		registry           *Registry
		registryConfigured bool
		err                error
	}
)

func (fn GatewayComponentFunc) ConfigureGateway(builder *GatewayBuilder) error {
	if fn == nil {
		return oops.In("vale").New("gateway component function cannot be nil")
	}
	if builder == nil {
		return oops.In("vale").New("gateway builder cannot be nil")
	}
	return fn(builder)
}

func NewGatewayBuilder(components ...GatewayComponent) *GatewayBuilder {
	return (&GatewayBuilder{}).WithComponents(components...)
}

func GatewayOptions(options ...Option) GatewayComponent {
	return GatewayComponentFunc(func(builder *GatewayBuilder) error {
		builder.WithOptions(options...)
		return nil
	})
}

func GatewayRegistry(registry *Registry) GatewayComponent {
	return GatewayComponentFunc(func(builder *GatewayBuilder) error {
		builder.WithRegistry(registry)
		return builder.Err()
	})
}

func GatewayExtensions(extensions ...Extension) GatewayComponent {
	return GatewayComponentFunc(func(builder *GatewayBuilder) error {
		builder.WithExtensions(extensions...)
		return builder.Err()
	})
}

func (b *GatewayBuilder) WithComponents(components ...GatewayComponent) *GatewayBuilder {
	if b == nil {
		return nil
	}
	b.ensureInit()
	for _, component := range components {
		if component == nil {
			continue
		}
		if err := component.ConfigureGateway(b); err != nil {
			b.recordError(oops.In("vale").Wrapf(err, "configure gateway component"))
			return b
		}
	}
	return b
}

func (b *GatewayBuilder) WithOptions(options ...Option) *GatewayBuilder {
	if b == nil {
		return nil
	}
	b.ensureInit()
	for _, option := range options {
		if option != nil {
			b.options.Add(option)
		}
	}
	return b
}

func (b *GatewayBuilder) WithOption(option Option) *GatewayBuilder {
	return b.WithOptions(option)
}

func (b *GatewayBuilder) WithRegistry(registry *Registry) *GatewayBuilder {
	if b == nil {
		return nil
	}
	b.ensureInit()
	if registry == nil {
		b.recordError(oops.In("vale").New("registry cannot be nil"))
		return b
	}
	b.registry = registry
	b.registryConfigured = true
	return b
}

func (b *GatewayBuilder) WithExtensions(extensions ...Extension) *GatewayBuilder {
	if b == nil {
		return nil
	}
	b.ensureInit()
	if err := b.registry.Use(extensions...); err != nil {
		b.recordError(err)
	}
	b.registryConfigured = true
	return b
}

func (b *GatewayBuilder) Registry() *Registry {
	if b == nil {
		return nil
	}
	b.ensureInit()
	b.registryConfigured = true
	return b.registry
}

func (b *GatewayBuilder) Options() *collectionlist.List[Option] {
	if b == nil || b.options == nil {
		return collectionlist.NewList[Option]()
	}
	copied := collectionlist.NewListWithCapacity[Option](b.options.Len())
	b.options.Range(func(_ int, option Option) bool {
		copied.Add(option)
		return true
	})
	return copied
}

func (b *GatewayBuilder) Err() error {
	if b == nil {
		return oops.In("vale").New("gateway builder cannot be nil")
	}
	return b.err
}

func (b *GatewayBuilder) Config() (Config, error) {
	return b.ConfigFrom(DefaultConfig())
}

func (b *GatewayBuilder) ConfigFrom(base Config) (Config, error) {
	if b == nil {
		return base, oops.In("vale").New("gateway builder cannot be nil")
	}
	b.ensureInit()
	if b.err != nil {
		return base, b.err
	}
	cfg := base
	if err := b.applyRegistry(&cfg); err != nil {
		return base, err
	}
	if err := b.applyOptions(&cfg); err != nil {
		return base, err
	}
	applyDefaultConfigSource(&cfg)
	return cfg, nil
}

func (b *GatewayBuilder) Build() (*Gateway, error) {
	cfg, err := b.Config()
	if err != nil {
		return nil, err
	}
	return NewFromConfig(cfg)
}

func (b *GatewayBuilder) BuildFromConfig(cfg Config) (*Gateway, error) {
	configured, err := b.ConfigFrom(cfg)
	if err != nil {
		return nil, err
	}
	return NewFromConfig(configured)
}

func (b *GatewayBuilder) MustBuild() *Gateway {
	gateway, err := b.Build()
	if err != nil {
		panic(err)
	}
	return gateway
}

func (b *GatewayBuilder) ensureInit() {
	if b.options == nil {
		b.options = collectionlist.NewList[Option]()
	}
	if b.registry == nil {
		b.registry = NewRegistry()
	}
}

func (b *GatewayBuilder) applyRegistry(cfg *Config) error {
	if !b.registryConfigured || b.registry == nil {
		return nil
	}
	if err := WithRegistry(b.registry)(cfg); err != nil {
		return oops.In("vale").Wrapf(err, "apply gateway registry")
	}
	return nil
}

func (b *GatewayBuilder) applyOptions(cfg *Config) error {
	var applyErr error
	b.options.Range(func(_ int, option Option) bool {
		if option == nil {
			return true
		}
		if err := option(cfg); err != nil {
			applyErr = oops.In("vale").Wrapf(err, "apply vale option")
			return false
		}
		return true
	})
	return applyErr
}

func (b *GatewayBuilder) recordError(err error) {
	if b.err == nil && err != nil {
		b.err = err
	}
}

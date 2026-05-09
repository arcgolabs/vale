package gateway

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	mergedprovider "github.com/arcgolabs/vale/provider/merged"
	staticconfigprovider "github.com/arcgolabs/vale/provider/staticconfig"
	"github.com/arcgolabs/vale/runtime"
	"github.com/samber/oops"
)

// Config holds construction-time settings for Gateway.
type Config struct {
	Watch              bool
	Cluster            ClusterFactory
	Logger             *slog.Logger
	EventBus           provider.EventBus
	Observability      observabilityx.Observability
	Provider           provider.SnapshotProvider
	ConfigSource       *collectionlist.List[provider.ConfigProvider]
	CertificateStorage certstore.Storage
	Metrics            MetricsFactory
	Middleware         *runtime.MiddlewareRegistry
	OnWatchError       func(error)
}

// DefaultConfig returns defaults used by New/NewFromConfig when paths or watch are unspecified.
func DefaultConfig() Config {
	return Config{
		Watch:        false,
		ConfigSource: collectionlist.NewList[provider.ConfigProvider](),
	}
}

// Gateway binds a SnapshotProvider-backed compiled runtime to HTTP servers: snapshot
// entrypoints plus admin (/admin/* and /metrics). Start and Stop each take a mutex; do not
// call them concurrently from multiple goroutines.
type Gateway struct {
	config   Config
	provider provider.SnapshotProvider
	logger   *slog.Logger
	events   provider.EventBus
	ownsBus  bool
	cluster  Cluster

	mu      sync.Mutex
	started bool

	runtime     *runtime.Gateway
	health      *runtime.HealthChecker
	watcher     io.Closer
	watchCancel context.CancelFunc
	servers     *collectionlist.List[*http.Server]
	reload      ReloadStatusView
}

// New applies options onto DefaultConfig then NewFromConfig.
func New(options ...Option) (*Gateway, error) {
	cfg := DefaultConfig()
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, oops.
				In("gateway").
				Wrapf(err, "apply gateway option")
		}
	}
	return NewFromConfig(cfg)
}

// NewDefault is equivalent to New() with defaults only (single default config path, watch on).
func NewDefault() (*Gateway, error) {
	return New()
}

// MustNew is like New but panics on option or construction error.
func MustNew(options ...Option) *Gateway {
	gateway, err := New(options...)
	if err != nil {
		panic(err)
	}
	return gateway
}

// NewFromConfig validates and fills defaults on cfg then constructs the Gateway. Use New
// to apply functional options first.
func NewFromConfig(cfg Config) (*Gateway, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	cfg.Observability = observabilityx.Normalize(cfg.Observability, cfg.Logger)
	ownsBus := false
	if cfg.EventBus == nil {
		cfg.EventBus = eventx.New()
		ownsBus = true
	}

	if cfg.Provider != nil && !cfg.ConfigSource.IsEmpty() {
		return nil, oops.
			In("gateway").
			New("cannot set both snapshot provider and config source providers")
	}
	if cfg.Provider != nil {
		provider.ApplyLogger(cfg.Provider, cfg.Logger)
	}

	if cfg.Provider == nil {
		configProviders := cfg.ConfigSource
		if configProviders.IsEmpty() {
			configProviders = collectionlist.NewList[provider.ConfigProvider](staticconfigprovider.New(config.Default()))
			cfg.Watch = false
		}
		configProviders.Range(func(_ int, configProvider provider.ConfigProvider) bool {
			provider.ApplyLogger(configProvider, cfg.Logger)
			return true
		})
		sourceList := collectionlist.NewListWithCapacity[mergedprovider.Source](configProviders.Len())
		configProviders.Range(func(index int, configProvider provider.ConfigProvider) bool {
			sourceList.Add(mergedprovider.Source{
				Name:     provider.ConfigProviderName(configProvider, fmt.Sprintf("source-%d", index)),
				Provider: configProvider,
			})
			return true
		})
		cfg.Provider = newMergedProvider(cfg.EventBus, cfg.Logger, sourceList, cfg.Middleware)
	}
	if cfg.OnWatchError == nil {
		cfg.OnWatchError = func(err error) {
			cfg.Logger.Error("watch error", "error", err)
		}
	}

	return &Gateway{
		config:   cfg,
		provider: cfg.Provider,
		logger:   cfg.Logger,
		events:   cfg.EventBus,
		ownsBus:  ownsBus,
	}, nil
}

func newMergedProvider(
	bus provider.EventBus,
	logger *slog.Logger,
	sources *collectionlist.List[mergedprovider.Source],
	middleware *runtime.MiddlewareRegistry,
) *mergedprovider.Provider {
	merged := mergedprovider.NewWithLoggerList(bus, logger, sources)
	if middleware != nil {
		merged.SetMiddlewareTypes(middleware.Names())
	}
	return merged
}

package gateway

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/observabilityx"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider"
	mergedprovider "github.com/arcgolabs/vela/provider/merged"
	staticconfigprovider "github.com/arcgolabs/vela/provider/staticconfig"
	"github.com/arcgolabs/vela/runtime"
	"github.com/samber/mo"
	"github.com/samber/oops"
	"golang.org/x/crypto/acme/autocert"
)

// Config holds construction-time settings for Gateway.
type Config struct {
	Watch         bool
	Cluster       ClusterFactory
	Logger        *slog.Logger
	EventBus      provider.EventBus
	Observability observabilityx.Observability
	Provider      provider.SnapshotProvider
	ConfigSource  *collectionlist.List[provider.ConfigProvider]
	Metrics       MetricsFactory
	Middleware    *runtime.MiddlewareRegistry
	OnWatchError  func(error)
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
		sources := sourceList.Values()
		cfg.Provider = mergedprovider.NewWithLogger(cfg.EventBus, cfg.Logger, sources...)
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

// Start loads the first snapshot, starts health checks, optionally watches for updates,
// and listens on snapshot entrypoints and admin address.
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.started {
		return oops.
			In("gateway").
			New("gateway already started")
	}
	g.logger.Info("gateway starting", "watch", g.config.Watch)

	if err := g.initializeCluster(); err != nil {
		return err
	}

	snapshot, err := g.loadInitialSnapshot(ctx)
	if err != nil {
		return err
	}
	g.runtime = runtime.NewGatewayWithMiddlewareRegistry(snapshot, g.logger, snapshot.AccessLogEnabled, g.buildMetrics(snapshot.MetricsEnabled), g.config.Middleware)
	g.publishClusterUpdate(snapshot)

	servers, listeners, entrypointNames, err := g.buildServers(ctx, snapshot)
	if err != nil {
		g.cleanupStartFailure(listeners)
		return oops.
			In("gateway").
			Wrapf(err, "build initial servers")
	}

	if g.config.Watch {
		if err := g.startWatcher(ctx); err != nil {
			g.cleanupStartFailure(listeners)
			return err
		}
	}

	g.startHealthChecker(ctx, snapshot)

	g.servers = servers
	g.serveServers(servers, listeners, entrypointNames)

	g.started = true
	g.logger.Info("gateway started", "entrypoints", snapshot.Entrypoints.Len(), "admin_addr", snapshot.AdminAddress)
	return nil
}

func (g *Gateway) initializeCluster() error {
	if g.config.Cluster == nil {
		return nil
	}
	cluster, err := g.config.Cluster(g.logger)
	if err != nil {
		return oops.
			In("gateway").
			Wrapf(err, "initialize cluster")
	}
	g.cluster = cluster
	if g.cluster != nil {
		g.logger.Info("cluster initialized", "status", g.cluster.Status())
	}
	return nil
}

func (g *Gateway) loadInitialSnapshot(ctx context.Context) (*runtime.CompiledSnapshot, error) {
	snapshot, err := g.provider.Load(ctx)
	if err != nil {
		g.shutdownClusterAfterInitialLoadFailure()
		g.logger.Error("initial snapshot load failed", "error", err)
		return nil, oops.
			In("gateway").
			Wrapf(err, "load initial snapshot")
	}
	g.logger.Info("initial snapshot loaded",
		"built_at", snapshot.BuiltAt,
		"entrypoints", snapshot.Entrypoints.Len(),
		"services", snapshot.Services.Len(),
		"routes", snapshot.Routes().Len(),
		"admin_addr", snapshot.AdminAddress,
		"proxy_engine", snapshot.ProxyEngine,
	)
	return snapshot, nil
}

func (g *Gateway) shutdownClusterAfterInitialLoadFailure() {
	if g.cluster == nil {
		return
	}
	if err := g.cluster.Shutdown(); err != nil {
		g.logger.Error("cluster shutdown after initial load failure failed", "error", err)
	}
	g.cluster = nil
}

// Stop shuts down watchers, health checks, all HTTP servers, cluster if enabled; closes
// the event bus only when Gateway created it.
func (g *Gateway) Stop(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.started {
		return nil
	}
	g.logger.Info("gateway stopping")

	g.stopWatcher()
	g.stopHealthChecker()
	g.stopServers(ctx)

	g.servers = nil
	g.runtime = nil
	g.started = false
	g.stopCluster()
	g.closeOwnedEventBus()
	g.logger.Info("gateway stopped")
	return nil
}

func (g *Gateway) stopWatcher() {
	if g.watchCancel != nil {
		g.watchCancel()
		g.watchCancel = nil
	}
	if g.watcher != nil {
		if err := g.watcher.Close(); err != nil {
			g.logger.Error("watcher close failed", "error", err)
		}
		g.watcher = nil
		g.logger.Info("watcher stopped")
	}
}

func (g *Gateway) stopHealthChecker() {
	if g.health != nil {
		g.health.Stop()
		g.health = nil
		g.logger.Info("health checker stopped")
	}
}

func (g *Gateway) stopServers(ctx context.Context) {
	if g.servers == nil {
		return
	}
	g.servers.Range(func(_ int, server *http.Server) bool {
		if err := server.Shutdown(ctx); err != nil {
			g.logger.Error("server shutdown failed", "addr", server.Addr, "error", err)
		}
		g.logger.Info("server stopped", "addr", server.Addr)
		return true
	})
}

func (g *Gateway) stopCluster() {
	if g.cluster != nil {
		if err := g.cluster.Shutdown(); err != nil {
			g.logger.Error("cluster shutdown failed", "error", err)
		}
		g.cluster = nil
		g.logger.Info("cluster stopped")
	}
}

func (g *Gateway) closeOwnedEventBus() {
	if g.ownsBus && g.events != nil {
		if err := g.events.Close(); err != nil {
			g.logger.Error("event bus close failed", "error", err)
		}
		g.logger.Info("event bus closed")
	}
}

func (g *Gateway) startWatcher(ctx context.Context) error {
	watchCtx, cancel := context.WithCancel(ctx)
	watcher, err := g.provider.Watch(watchCtx, func(snapshot *runtime.CompiledSnapshot) {
		g.applyReloadSnapshot(watchCtx, snapshot)
	}, func(watchErr error) {
		g.config.OnWatchError(watchErr)
	})
	if err != nil {
		cancel()
		g.logger.Error("watch setup failed", "error", err)
		return oops.
			In("gateway").
			Wrapf(err, "setup snapshot watcher")
	}
	g.watchCancel = cancel
	g.watcher = watcher
	g.logger.Info("watcher started")
	return nil
}

func (g *Gateway) startHealthChecker(ctx context.Context, snapshot *runtime.CompiledSnapshot) {
	interval := parseDurationDefault(snapshot.HealthInterval, 5*time.Second)
	timeout := parseDurationDefault(snapshot.HealthTimeout, 2*time.Second)
	g.health = runtime.NewHealthCheckerWithLogger(interval, timeout, g.logger)
	g.health.Start(ctx, g.runtime)
	g.logger.Info("health checker started", "interval", interval, "timeout", timeout)
}

func (g *Gateway) serveServers(servers *collectionlist.List[*http.Server], listeners *collectionlist.List[net.Listener], entrypointNames *collectionlist.List[string]) {
	entrypointNames.Range(func(index int, entrypoint string) bool {
		server, _ := servers.Get(index)
		listener, _ := listeners.Get(index)
		go g.listenEntrypoint(entrypoint, server, listener)
		return true
	})
	adminServer, _ := servers.Get(servers.Len() - 1)
	adminListener, _ := listeners.Get(listeners.Len() - 1)
	go g.listenAdmin(adminServer, adminListener)
}

// Events returns the event bus configured with WithEventBus or the internal instance.
func (g *Gateway) Events() provider.EventBus {
	return g.events
}

// Status returns a coarse snapshot-only map (started flag, counts, and cluster status when enabled).
func (g *Gateway) Status() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	status := map[string]any{
		"started": g.started,
	}
	if g.runtime != nil && g.runtime.Snapshot() != nil {
		snapshot := g.runtime.Snapshot()
		status["built_at"] = snapshot.BuiltAt
		status["entrypoints"] = snapshot.Entrypoints.Len()
		status["services"] = snapshot.Services.Len()
		status["routes"] = snapshot.Routes().Len()
	}
	if g.cluster != nil {
		status["cluster"] = g.cluster.Status()
	} else {
		status["cluster"] = map[string]any{"enabled": false}
	}
	return status
}

func (g *Gateway) listenEntrypoint(entrypoint string, server *http.Server, listener net.Listener) {
	g.logger.Info("entrypoint started", "entrypoint", entrypoint, "addr", server.Addr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		g.logger.Error("entrypoint crashed", "entrypoint", entrypoint, "error", err)
	}
}

func (g *Gateway) listenAdmin(server *http.Server, listener net.Listener) {
	g.logger.Info("admin started", "addr", server.Addr)
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		g.logger.Error("admin crashed", "error", err)
	}
}

func (g *Gateway) buildHTTPServer(address string, handler http.Handler, security runtime.SecurityRuntime) *http.Server {
	return &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: parseDurationDefault(security.ReadHeaderTimeout, 5*time.Second),
		ReadTimeout:       parseDurationDefault(security.ReadTimeout, 30*time.Second),
		WriteTimeout:      parseDurationDefault(security.WriteTimeout, 30*time.Second),
		IdleTimeout:       parseDurationDefault(security.IdleTimeout, 120*time.Second),
		MaxHeaderBytes:    maxInt(security.MaxHeaderBytes, 1<<20),
	}
}

func (g *Gateway) buildTLSConfig(tlsRuntime runtime.TLSRuntime) (*tls.Config, bool, error) {
	if !tlsRuntime.Enabled {
		return nil, false, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if tlsRuntime.CertFile != "" || tlsRuntime.KeyFile != "" {
		certificate, err := loadStaticTLSCertificate(tlsRuntime)
		if err != nil {
			return nil, false, err
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	if tlsRuntime.ACME.Enabled {
		applyACMETLSConfig(tlsConfig, tlsRuntime.ACME)
	}
	return tlsConfig, true, nil
}

func loadStaticTLSCertificate(tlsRuntime runtime.TLSRuntime) (tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(tlsRuntime.CertFile, tlsRuntime.KeyFile)
	if err != nil {
		return tls.Certificate{}, oops.
			In("gateway").
			With("cert_file", tlsRuntime.CertFile, "key_file", tlsRuntime.KeyFile).
			Wrapf(err, "load static tls certificate")
	}
	return certificate, nil
}

func applyACMETLSConfig(tlsConfig *tls.Config, acmeRuntime runtime.ACMERuntime) {
	manager := &autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Email:  acmeRuntime.Email,
	}
	if acmeRuntime.Domains != nil && !acmeRuntime.Domains.IsEmpty() {
		manager.HostPolicy = autocert.HostWhitelist(acmeRuntime.Domains.Values()...)
	}
	if acmeRuntime.CacheDir != "" {
		manager.Cache = autocert.DirCache(acmeRuntime.CacheDir)
	}
	acmeConfig := manager.TLSConfig()
	if len(tlsConfig.Certificates) == 0 {
		tlsConfig.GetCertificate = acmeConfig.GetCertificate
	}
	tlsConfig.NextProtos = acmeConfig.NextProtos
}

func (g *Gateway) cleanupStartFailure(listeners *collectionlist.List[net.Listener]) {
	g.closeStartFailureListeners(listeners)
	g.stopWatcher()
	g.stopHealthChecker()
	g.stopCluster()
	g.runtime = nil
	g.servers = nil
}

func (g *Gateway) closeStartFailureListeners(listeners *collectionlist.List[net.Listener]) {
	if listeners == nil {
		return
	}
	listeners.Range(func(_ int, listener net.Listener) bool {
		if listener == nil {
			return true
		}
		if err := listener.Close(); err != nil && g.logger != nil {
			g.logger.Error("listener close after start failure failed", "error", err)
		}
		return true
	})
}

func (g *Gateway) applyReloadSnapshot(ctx context.Context, snapshot *runtime.CompiledSnapshot) {
	if snapshot == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	current := g.runtime.Snapshot()
	if changed := staticRuntimeChanges(current, snapshot); !changed.IsEmpty() {
		g.logger.Info("snapshot contains static runtime changes; restarting servers",
			"fields", changed,
		)
		if g.events != nil {
			if err := g.events.Publish(ctx, StaticRuntimeConfigChangedEvent{
				Fields: changed,
			}); err != nil {
				g.logger.Error("publish static runtime change event failed", "error", err)
			}
		}
		if err := g.restartServersLocked(ctx, snapshot); err != nil {
			g.logger.Error("static runtime reload failed", "fields", changed, "error", err)
			g.config.OnWatchError(err)
		}
		return
	}
	g.runtime.Swap(snapshot)
	g.publishClusterUpdate(snapshot)
	g.runtime.ObserveReload("swapped")
	g.logger.Info("runtime snapshot swapped",
		"built_at", snapshot.BuiltAt,
		"entrypoints", snapshot.Entrypoints.Len(),
		"services", snapshot.Services.Len(),
		"routes", snapshot.Routes().Len(),
	)
}

func (g *Gateway) restartServersLocked(ctx context.Context, snapshot *runtime.CompiledSnapshot) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if g.health != nil {
		g.health.Stop()
		g.health = nil
	}
	g.servers.Range(func(_ int, server *http.Server) bool {
		if err := server.Shutdown(ctx); err != nil {
			g.logger.Error("server shutdown before restart failed", "addr", server.Addr, "error", err)
		}
		return true
	})
	g.servers = nil

	g.runtime = runtime.NewGatewayWithMiddlewareRegistry(snapshot, g.logger, snapshot.AccessLogEnabled, g.buildMetrics(snapshot.MetricsEnabled), g.config.Middleware)
	servers, listeners, entrypointNames, err := g.buildServers(ctx, snapshot)
	if err != nil {
		g.runtime.ObserveReload("failed")
		g.runtime = nil
		return oops.
			In("gateway").
			Wrapf(err, "build replacement servers")
	}
	g.servers = servers

	interval := parseDurationDefault(snapshot.HealthInterval, 5*time.Second)
	timeout := parseDurationDefault(snapshot.HealthTimeout, 2*time.Second)
	g.health = runtime.NewHealthCheckerWithLogger(interval, timeout, g.logger)
	g.health.Start(ctx, g.runtime)

	g.serveServers(servers, listeners, entrypointNames)
	g.publishClusterUpdate(snapshot)
	g.runtime.ObserveReload("restarted")
	g.logger.Info("servers restarted", "entrypoints", snapshot.Entrypoints.Len(), "admin_addr", snapshot.AdminAddress)
	return nil
}

func (g *Gateway) buildServers(ctx context.Context, snapshot *runtime.CompiledSnapshot) (*collectionlist.List[*http.Server], *collectionlist.List[net.Listener], *collectionlist.List[string], error) {
	servers := collectionlist.NewListWithCapacity[*http.Server](snapshot.Entrypoints.Len() + 1)
	listeners := collectionlist.NewListWithCapacity[net.Listener](snapshot.Entrypoints.Len() + 1)
	entrypointNames := collectionlist.NewListWithCapacity[string](snapshot.Entrypoints.Len())
	var buildErr error
	snapshot.Entrypoints.Range(func(entrypoint string, address string) bool {
		entrypointConfig, _ := snapshot.EntrypointConfigs.Get(entrypoint)
		server, listener, err := g.buildEntrypointServer(ctx, snapshot, entrypoint, address, entrypointConfig)
		if err != nil {
			closeListeners(listeners)
			buildErr = err
			return false
		}
		servers.Add(server)
		listeners.Add(listener)
		entrypointNames.Add(entrypoint)
		return true
	})
	if buildErr != nil {
		return nil, nil, nil, buildErr
	}

	adminServer, adminListener, err := g.buildAdminServer(ctx, snapshot)
	if err != nil {
		closeListeners(listeners)
		return nil, nil, nil, err
	}
	servers.Add(adminServer)
	listeners.Add(adminListener)
	return servers, listeners, entrypointNames, nil
}

func (g *Gateway) buildEntrypointServer(ctx context.Context, snapshot *runtime.CompiledSnapshot, entrypoint, address string, entrypointConfig runtime.EntrypointRuntime) (*http.Server, net.Listener, error) {
	server := g.buildHTTPServer(address, g.runtime.Handler(entrypoint), snapshot.Security)
	if tlsConfig, tlsEnabled, err := g.buildTLSConfig(entrypointConfig.TLS); err != nil {
		return nil, nil, oops.
			In("gateway").
			With("entrypoint", entrypoint, "address", address).
			Wrapf(err, "build entrypoint tls config")
	} else if tlsEnabled {
		server.TLSConfig = tlsConfig
	}
	listener, err := listenTCP(ctx, address)
	if err != nil {
		g.logger.Error("entrypoint listen failed", "entrypoint", entrypoint, "addr", address, "error", err)
		return nil, nil, oops.
			In("gateway").
			With("entrypoint", entrypoint, "address", address).
			Wrapf(err, "listen entrypoint")
	}
	if server.TLSConfig != nil {
		listener = tls.NewListener(listener, server.TLSConfig)
	}
	return server, listener, nil
}

func (g *Gateway) buildAdminServer(ctx context.Context, snapshot *runtime.CompiledSnapshot) (*http.Server, net.Listener, error) {
	server := g.buildHTTPServer(snapshot.AdminAddress, g.buildAdminMux(), snapshot.Security)
	listener, err := listenTCP(ctx, snapshot.AdminAddress)
	if err != nil {
		g.logger.Error("admin listen failed", "addr", snapshot.AdminAddress, "error", err)
		return nil, nil, oops.
			In("gateway").
			With("address", snapshot.AdminAddress).
			Wrapf(err, "listen admin")
	}
	return server, listener, nil
}

func listenTCP(ctx context.Context, address string) (net.Listener, error) {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(ctx, "tcp", address)
	if err != nil {
		return nil, oops.
			In("gateway").
			With("address", address).
			Wrapf(err, "listen tcp")
	}
	return listener, nil
}

func closeListeners(listeners *collectionlist.List[net.Listener]) {
	if listeners == nil {
		return
	}
	listeners.Range(func(_ int, listener net.Listener) bool {
		if listener != nil {
			if err := listener.Close(); err != nil {
				slog.Default().Error("listener close failed", "error", err)
			}
		}
		return true
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		slog.Default().Error("json response encode failed", "error", err)
	}
}

func parseDurationDefault(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	return mo.TupleToOption(duration, err == nil).OrElse(fallback)
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func (g *Gateway) publishClusterUpdate(snapshot *runtime.CompiledSnapshot) {
	if g.cluster == nil || !g.cluster.IsLeader() || snapshot == nil {
		return
	}
	payload := map[string]any{
		"type": "route_sync",
		"snapshot": map[string]any{
			"built_at":     snapshot.BuiltAt.UTC().Format(time.RFC3339Nano),
			"services":     snapshot.Services.Len(),
			"routes":       snapshot.Routes().Len(),
			"proxy_engine": snapshot.ProxyEngine,
		},
		"routes": adminRoutesView(snapshot, runtime.RouteFilter{}),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		g.logger.Error("raft payload marshal failed", "error", err)
		return
	}
	if err := g.cluster.Apply(data, 2*time.Second); err != nil {
		g.logger.Error("raft apply failed", "error", err)
	}
}

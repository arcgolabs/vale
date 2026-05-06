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
	ConfigSource  []provider.ConfigProvider
	Metrics       MetricsFactory
	OnWatchError  func(error)
}

// DefaultConfig returns defaults used by New/NewFromConfig when paths or watch are unspecified.
func DefaultConfig() Config {
	return Config{
		Watch: false,
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

	runtime *runtime.Gateway
	health  *runtime.HealthChecker
	watcher io.Closer
	servers []*http.Server
}

// New applies options onto DefaultConfig then NewFromConfig.
func New(options ...Option) (*Gateway, error) {
	cfg := DefaultConfig()
	for _, option := range options {
		if option == nil {
			continue
		}
		if err := option(&cfg); err != nil {
			return nil, err
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

	if cfg.Provider != nil && len(cfg.ConfigSource) > 0 {
		return nil, errors.New("cannot set both snapshot provider and config source providers")
	}
	if cfg.Provider != nil {
		provider.ApplyLogger(cfg.Provider, cfg.Logger)
	}

	if cfg.Provider == nil {
		configProviders := cfg.ConfigSource
		if len(configProviders) == 0 {
			configProviders = []provider.ConfigProvider{staticconfigprovider.New(config.Default())}
			cfg.Watch = false
		}
		for _, configProvider := range configProviders {
			provider.ApplyLogger(configProvider, cfg.Logger)
		}
		sourceList := collectionlist.NewListWithCapacity[mergedprovider.Source](len(configProviders))
		for index, configProvider := range configProviders {
			sourceList.Add(mergedprovider.Source{
				Name:     provider.ConfigProviderName(configProvider, fmt.Sprintf("source-%d", index)),
				Provider: configProvider,
			})
		}
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
		return errors.New("gateway already started")
	}
	g.logger.Info("gateway starting", "watch", g.config.Watch)

	if g.config.Cluster != nil {
		cluster, err := g.config.Cluster(g.logger)
		if err != nil {
			return err
		}
		g.cluster = cluster
		if g.cluster != nil {
			g.logger.Info("cluster initialized", "status", g.cluster.Status())
		}
	}

	snapshot, err := g.provider.Load(ctx)
	if err != nil {
		if g.cluster != nil {
			_ = g.cluster.Shutdown()
			g.cluster = nil
		}
		g.logger.Error("initial snapshot load failed", "error", err)
		return err
	}
	g.logger.Info("initial snapshot loaded",
		"built_at", snapshot.BuiltAt,
		"entrypoints", len(snapshot.Entrypoints),
		"services", len(snapshot.Services),
		"routes", len(snapshot.Routes()),
		"admin_addr", snapshot.AdminAddress,
		"proxy_engine", snapshot.ProxyEngine,
	)

	g.runtime = runtime.NewGateway(snapshot, g.logger, snapshot.AccessLogEnabled, g.buildMetrics(snapshot.MetricsEnabled))
	g.publishClusterUpdate(snapshot)

	servers := make([]*http.Server, 0, len(snapshot.Entrypoints)+1)
	listeners := make([]net.Listener, 0, len(snapshot.Entrypoints)+1)
	entrypointNames := make([]string, 0, len(snapshot.Entrypoints))
	for entrypoint, address := range snapshot.Entrypoints {
		entrypointConfig := snapshot.EntrypointConfigs[entrypoint]
		server := g.buildHTTPServer(address, g.runtime.Handler(entrypoint), snapshot.Security)
		if tlsConfig, tlsErr := g.buildTLSConfig(entrypointConfig.TLS); tlsErr != nil {
			g.cleanupStartFailure(listeners)
			return fmt.Errorf("build tls config for entrypoint %q: %w", entrypoint, tlsErr)
		} else {
			server.TLSConfig = tlsConfig
		}
		listener, listenErr := net.Listen("tcp", address)
		if listenErr != nil {
			g.cleanupStartFailure(listeners)
			g.logger.Error("entrypoint listen failed", "entrypoint", entrypoint, "addr", address, "error", listenErr)
			return fmt.Errorf("listen entrypoint %q on %s: %w", entrypoint, address, listenErr)
		}
		if server.TLSConfig != nil {
			listener = tls.NewListener(listener, server.TLSConfig)
		}
		servers = append(servers, server)
		listeners = append(listeners, listener)
		entrypointNames = append(entrypointNames, entrypoint)
	}

	adminServer := g.buildHTTPServer(snapshot.AdminAddress, g.buildAdminMux(), snapshot.Security)
	adminListener, listenErr := net.Listen("tcp", snapshot.AdminAddress)
	if listenErr != nil {
		g.cleanupStartFailure(listeners)
		g.logger.Error("admin listen failed", "addr", snapshot.AdminAddress, "error", listenErr)
		return fmt.Errorf("listen admin on %s: %w", snapshot.AdminAddress, listenErr)
	}
	servers = append(servers, adminServer)
	listeners = append(listeners, adminListener)

	if g.config.Watch {
		watcher, watchErr := g.provider.Watch(context.Background(), func(snapshot *runtime.CompiledSnapshot) {
			g.applyReloadSnapshot(snapshot)
		}, func(watchErr error) {
			g.config.OnWatchError(watchErr)
		})
		if watchErr != nil {
			g.cleanupStartFailure(listeners)
			g.logger.Error("watch setup failed", "error", watchErr)
			return watchErr
		}
		g.watcher = watcher
		g.logger.Info("watcher started")
	}

	interval := parseDurationDefault(snapshot.HealthInterval, 5*time.Second)
	timeout := parseDurationDefault(snapshot.HealthTimeout, 2*time.Second)
	g.health = runtime.NewHealthCheckerWithLogger(interval, timeout, g.logger)
	g.health.Start(g.runtime)
	g.logger.Info("health checker started", "interval", interval, "timeout", timeout)

	g.servers = servers
	for i, entrypoint := range entrypointNames {
		go g.listenEntrypoint(entrypoint, servers[i], listeners[i])
	}
	go g.listenAdmin(adminServer, listeners[len(listeners)-1])

	g.started = true
	g.logger.Info("gateway started", "entrypoints", len(snapshot.Entrypoints), "admin_addr", snapshot.AdminAddress)
	return nil
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

	if g.watcher != nil {
		_ = g.watcher.Close()
		g.watcher = nil
		g.logger.Info("watcher stopped")
	}
	if g.health != nil {
		g.health.Stop()
		g.health = nil
		g.logger.Info("health checker stopped")
	}
	for _, server := range g.servers {
		_ = server.Shutdown(ctx)
		g.logger.Info("server stopped", "addr", server.Addr)
	}

	g.servers = nil
	g.runtime = nil
	g.started = false
	if g.cluster != nil {
		_ = g.cluster.Shutdown()
		g.cluster = nil
		g.logger.Info("cluster stopped")
	}
	if g.ownsBus && g.events != nil {
		_ = g.events.Close()
		g.logger.Info("event bus closed")
	}
	g.logger.Info("gateway stopped")
	return nil
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
		status["entrypoints"] = len(snapshot.Entrypoints)
		status["services"] = len(snapshot.Services)
		status["routes"] = len(snapshot.Routes())
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

func (g *Gateway) buildTLSConfig(config runtime.TLSRuntime) (*tls.Config, error) {
	if !config.Enabled {
		return nil, nil
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if config.CertFile != "" || config.KeyFile != "" {
		certificate, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	if config.ACME.Enabled {
		manager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Email:      config.ACME.Email,
			HostPolicy: autocert.HostWhitelist(config.ACME.Domains...),
		}
		if config.ACME.CacheDir != "" {
			manager.Cache = autocert.DirCache(config.ACME.CacheDir)
		}
		acmeConfig := manager.TLSConfig()
		if len(tlsConfig.Certificates) == 0 {
			tlsConfig.GetCertificate = acmeConfig.GetCertificate
		}
		tlsConfig.NextProtos = acmeConfig.NextProtos
	}
	return tlsConfig, nil
}

func (g *Gateway) cleanupStartFailure(listeners []net.Listener) {
	for _, listener := range listeners {
		if listener != nil {
			_ = listener.Close()
		}
	}
	if g.watcher != nil {
		_ = g.watcher.Close()
		g.watcher = nil
	}
	if g.health != nil {
		g.health.Stop()
		g.health = nil
	}
	if g.cluster != nil {
		_ = g.cluster.Shutdown()
		g.cluster = nil
	}
	g.runtime = nil
	g.servers = nil
}

func (g *Gateway) applyReloadSnapshot(snapshot *runtime.CompiledSnapshot) {
	if snapshot == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	current := g.runtime.Snapshot()
	if changed := staticRuntimeChanges(current, snapshot); len(changed) > 0 {
		g.logger.Info("snapshot contains static runtime changes; restarting servers",
			"fields", changed,
		)
		if g.events != nil {
			_ = g.events.Publish(context.Background(), StaticRuntimeConfigChangedEvent{
				Fields: changed,
			})
		}
		if err := g.restartServersLocked(snapshot); err != nil {
			g.logger.Error("static runtime reload failed", "fields", changed, "error", err)
			g.config.OnWatchError(err)
		}
		return
	}
	g.runtime.Swap(snapshot)
	g.publishClusterUpdate(snapshot)
	g.logger.Info("runtime snapshot swapped",
		"built_at", snapshot.BuiltAt,
		"entrypoints", len(snapshot.Entrypoints),
		"services", len(snapshot.Services),
		"routes", len(snapshot.Routes()),
	)
}

func (g *Gateway) restartServersLocked(snapshot *runtime.CompiledSnapshot) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if g.health != nil {
		g.health.Stop()
		g.health = nil
	}
	for _, server := range g.servers {
		_ = server.Shutdown(ctx)
	}
	g.servers = nil

	g.runtime = runtime.NewGateway(snapshot, g.logger, snapshot.AccessLogEnabled, g.buildMetrics(snapshot.MetricsEnabled))
	servers, listeners, entrypointNames, err := g.buildServers(snapshot)
	if err != nil {
		g.runtime = nil
		return err
	}
	g.servers = servers

	interval := parseDurationDefault(snapshot.HealthInterval, 5*time.Second)
	timeout := parseDurationDefault(snapshot.HealthTimeout, 2*time.Second)
	g.health = runtime.NewHealthCheckerWithLogger(interval, timeout, g.logger)
	g.health.Start(g.runtime)

	for index, entrypoint := range entrypointNames {
		go g.listenEntrypoint(entrypoint, servers[index], listeners[index])
	}
	go g.listenAdmin(servers[len(servers)-1], listeners[len(listeners)-1])
	g.publishClusterUpdate(snapshot)
	g.logger.Info("servers restarted", "entrypoints", len(snapshot.Entrypoints), "admin_addr", snapshot.AdminAddress)
	return nil
}

func (g *Gateway) buildServers(snapshot *runtime.CompiledSnapshot) ([]*http.Server, []net.Listener, []string, error) {
	servers := make([]*http.Server, 0, len(snapshot.Entrypoints)+1)
	listeners := make([]net.Listener, 0, len(snapshot.Entrypoints)+1)
	entrypointNames := make([]string, 0, len(snapshot.Entrypoints))
	for entrypoint, address := range snapshot.Entrypoints {
		entrypointConfig := snapshot.EntrypointConfigs[entrypoint]
		server := g.buildHTTPServer(address, g.runtime.Handler(entrypoint), snapshot.Security)
		if tlsConfig, tlsErr := g.buildTLSConfig(entrypointConfig.TLS); tlsErr != nil {
			closeListeners(listeners)
			return nil, nil, nil, fmt.Errorf("build tls config for entrypoint %q: %w", entrypoint, tlsErr)
		} else {
			server.TLSConfig = tlsConfig
		}
		listener, listenErr := net.Listen("tcp", address)
		if listenErr != nil {
			closeListeners(listeners)
			return nil, nil, nil, fmt.Errorf("listen entrypoint %q on %s: %w", entrypoint, address, listenErr)
		}
		if server.TLSConfig != nil {
			listener = tls.NewListener(listener, server.TLSConfig)
		}
		servers = append(servers, server)
		listeners = append(listeners, listener)
		entrypointNames = append(entrypointNames, entrypoint)
	}

	adminServer := g.buildHTTPServer(snapshot.AdminAddress, g.buildAdminMux(), snapshot.Security)
	adminListener, listenErr := net.Listen("tcp", snapshot.AdminAddress)
	if listenErr != nil {
		closeListeners(listeners)
		return nil, nil, nil, fmt.Errorf("listen admin on %s: %w", snapshot.AdminAddress, listenErr)
	}
	servers = append(servers, adminServer)
	listeners = append(listeners, adminListener)
	return servers, listeners, entrypointNames, nil
}

func closeListeners(listeners []net.Listener) {
	for _, listener := range listeners {
		if listener != nil {
			_ = listener.Close()
		}
	}
}

func (g *Gateway) buildAdminMux() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", g.runtime.MetricsHandler())

	mux.HandleFunc("/admin/routes", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := g.runtime.Snapshot()
		if snapshot == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not ready"})
			return
		}
		writeJSON(w, http.StatusOK, snapshot.Routes())
	})

	mux.HandleFunc("/admin/services", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := g.runtime.Snapshot()
		if snapshot == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not ready"})
			return
		}
		writeJSON(w, http.StatusOK, snapshot.ServicesView())
	})

	mux.HandleFunc("/admin/endpoints", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := g.runtime.Snapshot()
		if snapshot == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime not ready"})
			return
		}

		endpointList := collectionlist.NewList[runtime.EndpointView]()
		for _, service := range snapshot.ServicesView() {
			endpointList.MergeSlice(service.Endpoints)
		}
		endpoints := endpointList.Values()
		writeJSON(w, http.StatusOK, endpoints)
	})
	mux.HandleFunc("/admin/cluster/status", func(w http.ResponseWriter, _ *http.Request) {
		if g.cluster == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"enabled": false,
			})
			return
		}
		writeJSON(w, http.StatusOK, g.cluster.Status())
	})
	mux.HandleFunc("/admin/cluster/peers", func(w http.ResponseWriter, _ *http.Request) {
		if g.cluster == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		peers, err := g.cluster.Peers()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, peers)
	})
	mux.HandleFunc("/admin/cluster/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if g.cluster == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "raft is not enabled"})
			return
		}
		if !g.cluster.IsLeader() {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "only leader can join peers"})
			return
		}
		var req struct {
			ID      string `json:"id"`
			Address string `json:"address"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := g.cluster.AddVoter(req.ID, req.Address, 5*time.Second); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/admin/cluster/leave", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if g.cluster == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "raft is not enabled"})
			return
		}
		if !g.cluster.IsLeader() {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "only leader can remove peers"})
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := g.cluster.RemoveServer(req.ID, 5*time.Second); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func parseDurationDefault(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	return mo.TupleToOption(duration, err == nil).OrElse(fallback)
}

func maxInt(value int, fallback int) int {
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
		"built_at":  snapshot.BuiltAt.UTC().Format(time.RFC3339Nano),
		"services":  len(snapshot.Services),
		"routes":    len(snapshot.Routes()),
		"proxy_eng": snapshot.ProxyEngine,
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

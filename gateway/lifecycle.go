package gateway

import (
	"context"
	"net"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/certstore"
	"github.com/arcgolabs/vale/runtime"
	"github.com/samber/oops"
)

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
	g.recordInitialSnapshotLocked(snapshot)
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
		g.configureClusterCertificateStorage()
		g.logger.Info("cluster initialized", "status", g.cluster.Status())
	}
	return nil
}

func (g *Gateway) configureClusterCertificateStorage() {
	if g.config.CertificateStorage != nil {
		return
	}
	client, ok := g.cluster.(certstore.RaftClient)
	if !ok {
		return
	}
	g.config.CertificateStorage = certstore.NewRaftStorage(certstore.RaftStorageConfig{
		Client: client,
		Group:  ClusterGroupCertificates,
	})
	g.logger.Info("cluster certificate storage enabled", "group", ClusterGroupCertificates)
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
		g.recordReloadError(watchErr)
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

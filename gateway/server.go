package gateway

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/runtime"
	"github.com/samber/oops"
)

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

func (g *Gateway) restartServersLocked(ctx context.Context, snapshot *runtime.CompiledSnapshot) error {
	restartCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if g.health != nil {
		g.health.Stop()
		g.health = nil
	}
	g.servers.Range(func(_ int, server *http.Server) bool {
		if err := server.Shutdown(restartCtx); err != nil {
			g.logger.Error("server shutdown before restart failed", "addr", server.Addr, "error", err)
		}
		return true
	})
	g.servers = nil

	g.runtime = runtime.NewGatewayWithMiddlewareRegistry(snapshot, g.logger, snapshot.AccessLogEnabled, g.buildMetrics(snapshot.MetricsEnabled), g.config.Middleware)
	servers, listeners, entrypointNames, err := g.buildServers(restartCtx, snapshot)
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

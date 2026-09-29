package runtime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
)

type closeReader interface {
	CloseRead() error
}

type closeWriter interface {
	CloseWrite() error
}

func (g *Gateway) ProxyTCP(ctx context.Context, entrypoint string, downstream net.Conn) {
	if ctx == nil || downstream == nil {
		return
	}
	defer closeTCPConnection(ctx, g.logger, downstream, "close downstream")

	snapshot := g.current.Load()
	route := tcpRouteForEntrypoint(snapshot, entrypoint)
	if route == nil || route.Service == nil {
		return
	}
	endpoint, err := route.Service.Pick()
	if err != nil || endpoint == nil || endpoint.Address == "" {
		return
	}

	var dialer net.Dialer
	upstream, err := dialer.DialContext(ctx, "tcp", endpoint.Address)
	if err != nil {
		endpoint.Healthy.Store(false)
		return
	}
	defer closeTCPConnection(ctx, g.logger, upstream, "close upstream")
	endpoint.Healthy.Store(true)

	proxyTCPStreams(ctx, g.logger, downstream, upstream)
}

func tcpRouteForEntrypoint(snapshot *CompiledSnapshot, entrypoint string) *CompiledTCPRoute {
	if snapshot == nil || snapshot.TCPRoutes == nil {
		return nil
	}
	route, _ := snapshot.TCPRoutes.Get(entrypoint)
	return route
}

func proxyTCPStreams(ctx context.Context, logger *slog.Logger, downstream, upstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go copyTCPHalf(ctx, logger, &wg, upstream, downstream)
	go copyTCPHalf(ctx, logger, &wg, downstream, upstream)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		closeTCPConnection(ctx, logger, downstream, "cancel downstream")
		closeTCPConnection(ctx, logger, upstream, "cancel upstream")
	case <-done:
	}
}

func copyTCPHalf(ctx context.Context, logger *slog.Logger, wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, err := io.Copy(dst, src)
	reportTCPProxyError(ctx, logger, "copy stream", err)
	closeTCPWrite(ctx, logger, dst)
	closeTCPRead(ctx, logger, src)
}

func closeTCPWrite(ctx context.Context, logger *slog.Logger, conn net.Conn) {
	if closer, ok := conn.(closeWriter); ok {
		reportTCPProxyError(ctx, logger, "close write", closer.CloseWrite())
		return
	}
	closeTCPConnection(ctx, logger, conn, "close write connection")
}

func closeTCPRead(ctx context.Context, logger *slog.Logger, conn net.Conn) {
	if closer, ok := conn.(closeReader); ok {
		reportTCPProxyError(ctx, logger, "close read", closer.CloseRead())
	}
}

func closeTCPConnection(ctx context.Context, logger *slog.Logger, conn net.Conn, operation string) {
	reportTCPProxyError(ctx, logger, operation, conn.Close())
}

func reportTCPProxyError(ctx context.Context, logger *slog.Logger, operation string, err error) {
	if logger == nil || err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
		return
	}
	logger.DebugContext(ctx, "tcp proxy operation failed", "operation", operation, "error", err)
}

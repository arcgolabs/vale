package runtime

import (
	"context"
	"io"
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
	if downstream == nil {
		return
	}
	defer downstream.Close()
	if ctx == nil {
		ctx = context.Background()
	}

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
	defer upstream.Close()
	endpoint.Healthy.Store(true)

	proxyTCPStreams(ctx, downstream, upstream)
}

func tcpRouteForEntrypoint(snapshot *CompiledSnapshot, entrypoint string) *CompiledTCPRoute {
	if snapshot == nil || snapshot.TCPRoutes == nil {
		return nil
	}
	route, _ := snapshot.TCPRoutes.Get(entrypoint)
	return route
}

func proxyTCPStreams(ctx context.Context, downstream, upstream net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go copyTCPHalf(&wg, upstream, downstream)
	go copyTCPHalf(&wg, downstream, upstream)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		_ = downstream.Close()
		_ = upstream.Close()
	case <-done:
	}
}

func copyTCPHalf(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	closeTCPWrite(dst)
	closeTCPRead(src)
}

func closeTCPWrite(conn net.Conn) {
	if closer, ok := conn.(closeWriter); ok {
		_ = closer.CloseWrite()
		return
	}
	_ = conn.Close()
}

func closeTCPRead(conn net.Conn) {
	if closer, ok := conn.(closeReader); ok {
		_ = closer.CloseRead()
	}
}

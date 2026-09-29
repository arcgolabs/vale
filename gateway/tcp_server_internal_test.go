package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

func TestTCPServerShutdownWrapsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	server := &tcpServer{conns: mapping.NewMap[net.Conn, struct{}]()}
	server.wg.Add(1)

	err := server.Shutdown(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown error = %v, want context cancellation", err)
	}
	if !strings.Contains(err.Error(), "wait for tcp server shutdown") {
		t.Fatalf("Shutdown error = %q, want wrapped shutdown context", err)
	}
}

func TestTCPServerShutdownReturnsConnectionCloseError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("connection close failed")
	conn := &closeErrorConn{closeErr: wantErr}
	server := &tcpServer{conns: mapping.NewMap[net.Conn, struct{}]()}
	server.conns.Set(conn, struct{}{})

	err := server.Shutdown(t.Context())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Shutdown error = %v, want connection close error", err)
	}
	if !strings.Contains(err.Error(), "close active tcp connection") {
		t.Fatalf("Shutdown error = %q, want wrapped connection close context", err)
	}
}

func TestCleanupStartFailureUsesCallerContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	server := &tcpServer{conns: mapping.NewMap[net.Conn, struct{}]()}
	server.wg.Add(1)
	gateway := &Gateway{logger: slog.New(slog.DiscardHandler)}

	startedAt := time.Now()
	gateway.cleanupStartFailure(
		ctx,
		nil,
		collectionlist.NewList(server),
	)
	if elapsed := time.Since(startedAt); elapsed > 100*time.Millisecond {
		t.Fatalf("cleanup took %s, want caller cancellation to stop it promptly", elapsed)
	}
}

type closeErrorConn struct {
	net.Conn
	closeErr error
}

func (c *closeErrorConn) Close() error {
	return c.closeErr
}

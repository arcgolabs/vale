package runtime_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

var errTCPProxyTestRead = errors.New("test downstream read failed")

func TestGatewayTCPProxyLogsCopyFailureWithInjectedLogger(t *testing.T) {
	t.Parallel()

	listener := listenForTCPProxyTest(t)
	acceptErr := acceptAndCloseTCPProxyTestConnection(listener)

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	service := valeruntime.NewTCPService(
		"backend",
		"round_robin",
		valeruntime.NewTCPEndpoint(listener.Addr().String(), 1),
	)
	snapshot := valeruntime.NewSnapshot().
		AddTCPService(service).
		AddTCPRoute(valeruntime.NewTCPRoute("backend", "tcp", service))
	gateway := valeruntime.NewGateway(snapshot, logger, false, valeruntime.NewNoopMetrics())

	gateway.ProxyTCP(t.Context(), "tcp", &failingTCPProxyConn{})

	if err := <-acceptErr; err != nil {
		t.Fatalf("accept backend connection: %v", err)
	}
	output := logs.String()
	if !strings.Contains(output, "tcp proxy operation failed") || !strings.Contains(output, errTCPProxyTestRead.Error()) {
		t.Fatalf("logs = %q, want injected tcp proxy copy failure", output)
	}
}

func listenForTCPProxyTest(t *testing.T) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for tcp proxy test: %v", err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Errorf("close tcp proxy test listener: %v", err)
		}
	})
	return listener
}

func acceptAndCloseTCPProxyTestConnection(listener net.Listener) <-chan error {
	result := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		result <- connection.Close()
	}()
	return result
}

type failingTCPProxyConn struct{}

func (*failingTCPProxyConn) Read([]byte) (int, error)         { return 0, errTCPProxyTestRead }
func (*failingTCPProxyConn) Write(data []byte) (int, error)   { return len(data), nil }
func (*failingTCPProxyConn) Close() error                     { return nil }
func (*failingTCPProxyConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*failingTCPProxyConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*failingTCPProxyConn) SetDeadline(time.Time) error      { return nil }
func (*failingTCPProxyConn) SetReadDeadline(time.Time) error  { return nil }
func (*failingTCPProxyConn) SetWriteDeadline(time.Time) error { return nil }
func (*failingTCPProxyConn) CloseRead() error                 { return nil }
func (*failingTCPProxyConn) CloseWrite() error                { return nil }

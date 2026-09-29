package gateway_test

import (
	"bufio"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/arcgolabs/vale/gateway"
	"github.com/arcgolabs/vale/runtime"
)

func TestGatewayTCPProxyEntrypoint(t *testing.T) {
	t.Parallel()

	backendAddr := startTCPEchoBackend(t)
	entryAddr := freeAddr(t)
	adminAddr := freeAddr(t)
	service := runtime.NewTCPService("echo", "round_robin", runtime.NewTCPEndpoint(backendAddr, 1))
	snapshot := runtime.NewSnapshot().
		AddEntrypoint("tcp", entryAddr, runtime.EntrypointRuntime{
			Name:     "tcp",
			Address:  entryAddr,
			Protocol: runtime.EntrypointProtocolTCP,
		}).
		AddTCPService(service).
		AddTCPRoute(runtime.NewTCPRoute("echo", "tcp", service)).
		BuildMatchers()
	snapshot.AdminAddress = adminAddr

	g, err := gateway.New(
		gateway.WithStaticSnapshot(snapshot),
		gateway.WithWatch(false),
		gateway.WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}
	startErr := g.Start(t.Context())
	if startErr != nil {
		t.Fatal(startErr)
	}
	defer stopGateway(t, g)

	dialer := net.Dialer{Timeout: time.Second}
	conn, dialErr := dialer.DialContext(t.Context(), "tcp", entryAddr)
	if dialErr != nil {
		t.Fatal(dialErr)
	}
	defer closeConn(t, conn)
	if _, writeErr := conn.Write([]byte("ping\n")); writeErr != nil {
		t.Fatal(writeErr)
	}
	got, readErr := bufio.NewReader(conn).ReadString('\n')
	if readErr != nil {
		t.Fatal(readErr)
	}
	if got != "ping\n" {
		t.Fatalf("tcp response = %q, want ping", got)
	}
}

func startTCPEchoBackend(t *testing.T) string {
	t.Helper()

	listener := listenOnLocalhost(t)
	var handlers sync.WaitGroup
	handlers.Add(1)
	t.Cleanup(func() {
		stopTCPEchoBackend(t, listener, &handlers)
	})
	go serveTCPEchoBackend(t, listener, &handlers)
	return listener.Addr().String()
}

func stopTCPEchoBackend(t *testing.T, listener net.Listener, handlers *sync.WaitGroup) {
	t.Helper()
	if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
		t.Errorf("close TCP echo listener: %v", closeErr)
	}
	handlers.Wait()
}

func serveTCPEchoBackend(t *testing.T, listener net.Listener, handlers *sync.WaitGroup) {
	t.Helper()
	defer handlers.Done()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				t.Errorf("accept TCP echo connection: %v", err)
			}
			return
		}
		handlers.Add(1)
		go echoTCPConnection(t, conn, handlers)
	}
}

func echoTCPConnection(t *testing.T, conn net.Conn, handlers *sync.WaitGroup) {
	t.Helper()
	defer handlers.Done()
	defer func() {
		if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			t.Errorf("close TCP echo connection: %v", closeErr)
		}
	}()
	if _, copyErr := io.Copy(conn, conn); copyErr != nil && !errors.Is(copyErr, net.ErrClosed) {
		t.Errorf("copy TCP echo stream: %v", copyErr)
	}
}

package gateway_test

import (
	"bufio"
	"errors"
	"io"
	"net"
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
	if err := g.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer stopGateway(t, g)

	conn, err := net.DialTimeout("tcp", entryAddr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConn(t, conn)
	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if got != "ping\n" {
		t.Fatalf("tcp response = %q, want ping", got)
	}
}

func startTCPEchoBackend(t *testing.T) string {
	t.Helper()

	listener := listenOnLocalhost(t)
	t.Cleanup(func() {
		_ = listener.Close()
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}(conn)
		}
	}()
	return listener.Addr().String()
}

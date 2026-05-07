package gateway_test

import (
	"crypto/tls"
	"strings"
	"testing"

	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/runtime"
)

func TestStartReturnsEntrypointListenError(t *testing.T) {
	t.Parallel()

	occupied := listenOnLocalhost(t)
	defer closeListener(t, occupied)

	g, err := gateway.New(
		gateway.WithStaticSnapshot(emptySnapshot(occupied.Addr().String(), "127.0.0.1:0")),
		gateway.WithWatch(false),
		gateway.WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = g.Start(t.Context())
	if err == nil {
		t.Fatal("Start returned nil error, want listen error")
	}
	if !strings.Contains(err.Error(), "listen entrypoint") {
		t.Fatalf("Start error = %q, want entrypoint listen error", err.Error())
	}
}

func TestStartReturnsAdminListenError(t *testing.T) {
	t.Parallel()

	occupied := listenOnLocalhost(t)
	defer closeListener(t, occupied)

	g, err := gateway.New(
		gateway.WithStaticSnapshot(emptySnapshot("127.0.0.1:0", occupied.Addr().String())),
		gateway.WithWatch(false),
		gateway.WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = g.Start(t.Context())
	if err == nil {
		t.Fatal("Start returned nil error, want listen error")
	}
	if !strings.Contains(err.Error(), "listen admin") {
		t.Fatalf("Start error = %q, want admin listen error", err.Error())
	}
}

func TestAdminAPIWritesPlainJSONViews(t *testing.T) {
	t.Parallel()

	adminAddr := freeAddr(t)
	snapshot := defaultSnapshotOnFreePorts(t, adminAddr)
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

	assertAdminJSONLen[runtime.RouteView](t, adminAddr, "/admin/routes", 1)
	assertAdminJSONLen[runtime.RouteView](t, adminAddr, "/admin/routes?service=missing", 0)
	services := assertAdminJSONLen[adminServiceView](t, adminAddr, "/admin/services", 1)
	if len(services) != 1 || len(services[0].Endpoints) != 1 {
		t.Fatalf("services = %#v, want one service with one endpoint", services)
	}
	assertAdminJSONLen[runtime.EndpointView](t, adminAddr, "/admin/endpoints", 1)
}

func TestStartLoadsStaticTLSCertificate(t *testing.T) {
	t.Parallel()

	certFile, keyFile, certPEM := writeTestCertificate(t)
	entryAddr := freeAddr(t)
	snapshot := emptySnapshot(entryAddr, freeAddr(t))
	entrypoint, _ := snapshot.EntrypointConfigs.Get("web")
	entrypoint.TLS = runtime.TLSRuntime{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
	}
	snapshot.EntrypointConfigs.Set("web", entrypoint)

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

	conn := dialTLS(t, entryAddr, certPEM)
	defer closeConn(t, conn)
	if conn.ConnectionState().Version < tls.VersionTLS12 {
		t.Fatalf("tls version = %d, want at least TLS 1.2", conn.ConnectionState().Version)
	}
}

func TestStartReportsMissingStaticTLSCertificate(t *testing.T) {
	t.Parallel()

	snapshot := emptySnapshot(freeAddr(t), freeAddr(t))
	entrypoint, _ := snapshot.EntrypointConfigs.Get("web")
	entrypoint.TLS = runtime.TLSRuntime{
		Enabled:  true,
		CertFile: "missing-cert.pem",
		KeyFile:  "missing-key.pem",
	}
	snapshot.EntrypointConfigs.Set("web", entrypoint)

	g, err := gateway.New(
		gateway.WithStaticSnapshot(snapshot),
		gateway.WithWatch(false),
		gateway.WithLogger(discardLogger()),
	)
	if err != nil {
		t.Fatal(err)
	}

	err = g.Start(t.Context())
	if err == nil {
		t.Fatal("Start returned nil error for missing certificate files")
	}
	if !strings.Contains(err.Error(), "load static tls certificate") {
		t.Fatalf("Start error = %q, want static certificate load error", err.Error())
	}
}

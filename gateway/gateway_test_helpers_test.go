package gateway_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/arcgolabs/vale/compiler"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/gateway"
	"github.com/arcgolabs/vale/runtime"
)

type adminServiceView struct {
	Name      string                 `json:"name"`
	Strategy  string                 `json:"strategy"`
	Endpoints []runtime.EndpointView `json:"endpoints"`
}

func emptySnapshot(entryAddr, adminAddr string) *runtime.CompiledSnapshot {
	snapshot := runtime.NewSnapshot().
		AddEntrypoint("web", entryAddr, runtime.EntrypointRuntime{
			Name:    "web",
			Address: entryAddr,
		}).
		BuildMatchers()
	snapshot.AdminAddress = adminAddr
	return snapshot
}

func defaultSnapshotOnFreePorts(t *testing.T, adminAddr string) *runtime.CompiledSnapshot {
	t.Helper()

	snapshot, err := compiler.Compile(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	entryAddr := freeAddr(t)
	snapshot.AdminAddress = adminAddr
	snapshot.Entrypoints.Set("web", entryAddr)
	entrypoint, ok := snapshot.EntrypointConfigs.Get("web")
	if !ok {
		t.Fatal("default snapshot missing web entrypoint config")
	}
	entrypoint.Address = entryAddr
	snapshot.EntrypointConfigs.Set("web", entrypoint)
	return snapshot
}

func assertAdminJSONLen[T any](t *testing.T, adminAddr, path string, want int) []T {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+adminAddr+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	var rows []T
	if err := json.NewDecoder(response.Body).Decode(&rows); err != nil {
		t.Fatalf("%s json decode failed: %v", path, err)
	}
	if len(rows) != want {
		t.Fatalf("%s len = %d, want %d", path, len(rows), want)
	}
	return rows
}

func listenOnLocalhost(t *testing.T) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return listener
}

func freeAddr(t *testing.T) string {
	t.Helper()

	listener := listenOnLocalhost(t)
	addr := listener.Addr().String()
	closeListener(t, listener)
	return addr
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func stopGateway(t *testing.T, g *gateway.Gateway) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := g.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func dialTLS(t *testing.T, addr string, certPEM []byte) *tls.Conn {
	t.Helper()

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to append certificate to root pool")
	}
	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: time.Second},
		Config: &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
			ServerName: "localhost",
		},
	}
	conn, err := dialer.DialContext(t.Context(), "tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		closeConn(t, conn)
		t.Fatal("connection is not a TLS connection")
	}
	return tlsConn
}

func writeTestCertificate(t *testing.T) (string, string, []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})
	dir := t.TempDir()
	certFile := dir + "/cert.pem"
	keyFile := dir + "/key.pem"
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile, certPEM
}

func closeListener(t *testing.T, listener net.Listener) {
	t.Helper()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
}

func closeConn(t *testing.T, conn net.Conn) {
	t.Helper()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
}

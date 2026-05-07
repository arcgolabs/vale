package gateway

import (
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
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/vela/compiler"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/runtime"
)

func TestStartReturnsEntrypointListenError(t *testing.T) {
	t.Parallel()

	occupied := listenOnLocalhost(t)
	defer func() {
		if err := occupied.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	g, err := New(
		WithStaticSnapshot(&runtime.CompiledSnapshot{
			Entrypoints:  mapping.NewMapFrom(map[string]string{"web": occupied.Addr().String()}),
			AdminAddress: "127.0.0.1:0",
		}),
		WithWatch(false),
		WithLogger(discardLogger()),
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
	defer func() {
		if err := occupied.Close(); err != nil {
			t.Fatal(err)
		}
	}()

	g, err := New(
		WithStaticSnapshot(&runtime.CompiledSnapshot{
			Entrypoints:  mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:0"}),
			AdminAddress: occupied.Addr().String(),
		}),
		WithWatch(false),
		WithLogger(discardLogger()),
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

func TestStaticRuntimeChanges(t *testing.T) {
	t.Parallel()

	current := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8080"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19090",
		AccessLogEnabled:  true,
		MetricsEnabled:    true,
		HealthInterval:    "5s",
		HealthTimeout:     "2s",
	}
	next := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8081"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19091",
		AccessLogEnabled:  false,
		MetricsEnabled:    false,
		HealthInterval:    "10s",
		HealthTimeout:     "3s",
	}

	got := staticRuntimeChanges(current, next)
	want := []string{
		"access_log_enabled",
		"admin_address",
		"entrypoints",
		"health_interval",
		"health_timeout",
		"metrics_enabled",
	}
	if !slices.Equal(got.Values(), want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestStaticRuntimeChangesIgnoresDynamicSnapshotFields(t *testing.T) {
	t.Parallel()

	current := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8080"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19090",
		AccessLogEnabled:  true,
		MetricsEnabled:    true,
		HealthInterval:    "5s",
		HealthTimeout:     "2s",
		ProxyEngine:       "",
	}
	next := &runtime.CompiledSnapshot{
		Entrypoints:       mapping.NewMapFrom(map[string]string{"web": "127.0.0.1:8080"}),
		EntrypointConfigs: mapping.NewMap[string, runtime.EntrypointRuntime](),
		AdminAddress:      "127.0.0.1:19090",
		AccessLogEnabled:  true,
		MetricsEnabled:    true,
		HealthInterval:    "5s",
		HealthTimeout:     "2s",
		ProxyEngine:       "oxy",
	}

	got := staticRuntimeChanges(current, next)
	if !got.IsEmpty() {
		t.Fatalf("changes = %v, want none", got)
	}
}

func TestAdminAPIWritesPlainJSONViews(t *testing.T) {
	t.Parallel()

	snapshot, err := compiler.Compile(config.Default())
	if err != nil {
		t.Fatal(err)
	}
	gateway := &Gateway{
		runtime: runtime.NewGateway(snapshot, discardLogger(), false, runtime.NewNoopMetrics()),
		logger:  discardLogger(),
	}
	mux := gateway.buildAdminMux()

	assertAdminJSONLen[runtime.RouteView](t, mux, "/admin/routes", 1)
	assertAdminJSONLen[runtime.RouteView](t, mux, "/admin/routes?service=missing", 0)
	services := assertAdminJSONLen[adminServiceView](t, mux, "/admin/services", 1)
	if len(services) != 1 || len(services[0].Endpoints) != 1 {
		t.Fatalf("services = %#v, want one service with one endpoint", services)
	}
	assertAdminJSONLen[runtime.EndpointView](t, mux, "/admin/endpoints", 1)
}

func assertAdminJSONLen[T any](t *testing.T, handler http.Handler, path string, want int) []T {
	t.Helper()

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody))
	var rows []T
	if err := json.Unmarshal(recorder.Body.Bytes(), &rows); err != nil {
		t.Fatalf("%s json decode failed: %v; body=%s", path, err, recorder.Body.String())
	}
	if len(rows) != want {
		t.Fatalf("%s len = %d, want %d", path, len(rows), want)
	}
	return rows
}

func TestBuildTLSConfigLoadsStaticCertificate(t *testing.T) {
	t.Parallel()

	certFile, keyFile := writeTestCertificate(t)
	tlsConfig, tlsEnabled, err := (&Gateway{}).buildTLSConfig(runtime.TLSRuntime{
		Enabled:  true,
		CertFile: certFile,
		KeyFile:  keyFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !tlsEnabled {
		t.Fatal("tls was not enabled")
	}
	if tlsConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("min tls version = %d, want TLS 1.2", tlsConfig.MinVersion)
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Fatalf("certificates = %d, want 1", len(tlsConfig.Certificates))
	}
}

func TestBuildTLSConfigReportsMissingStaticCertificate(t *testing.T) {
	t.Parallel()

	_, _, err := (&Gateway{}).buildTLSConfig(runtime.TLSRuntime{
		Enabled:  true,
		CertFile: "missing-cert.pem",
		KeyFile:  "missing-key.pem",
	})
	if err == nil {
		t.Fatal("buildTLSConfig returned nil error for missing certificate files")
	}
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

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func writeTestCertificate(t *testing.T) (string, string) {
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
	return certFile, keyFile
}

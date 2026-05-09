package gateway

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/certstore"
	"github.com/caddyserver/certmagic"
)

func TestACMEDomainAllowed(t *testing.T) {
	t.Parallel()

	domains := collectionlist.NewList("api.example.com", "*.apps.example.com")
	tests := []struct {
		name       string
		serverName string
		want       bool
	}{
		{name: "exact", serverName: "API.EXAMPLE.COM", want: true},
		{name: "wildcard single label", serverName: "tenant.apps.example.com", want: true},
		{name: "wildcard root excluded", serverName: "apps.example.com", want: false},
		{name: "wildcard multi label excluded", serverName: "a.b.apps.example.com", want: false},
		{name: "unknown", serverName: "api.other.example.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := acmeDomainAllowed(tt.serverName, domains); got != tt.want {
				t.Fatalf("acmeDomainAllowed(%q) = %t, want %t", tt.serverName, got, tt.want)
			}
		})
	}
}

func TestMergeTLSNextProtosKeepsOrder(t *testing.T) {
	t.Parallel()

	got := mergeTLSNextProtos([]string{"h2", "http/1.1"}, []string{"acme-tls/1", "h2", ""})
	want := []string{"h2", "http/1.1", "acme-tls/1"}
	if len(got) != len(want) {
		t.Fatalf("merged protos = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("merged protos = %v, want %v", got, want)
		}
	}
}

func TestCertMagicStorageAdapterUsesCertificateStorage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	adapter := newTestCertMagicStorage(t)

	if err := adapter.Store(ctx, "certificates/local/example.com/example.com.crt", []byte("cert")); err != nil {
		t.Fatalf("store through adapter: %v", err)
	}
	assertStorageValue(t, adapter, "certificates/local/example.com/example.com.crt", "cert")
	assertStorageKeys(t, adapter, "certificates", true, []string{
		"certificates/local",
		"certificates/local/example.com",
		"certificates/local/example.com/example.com.crt",
	})
	assertTerminalStat(t, adapter, "certificates/local/example.com/example.com.crt", 4)
}

func TestCertMagicStorageAdapterPreservesNotExist(t *testing.T) {
	t.Parallel()

	adapter := newTestCertMagicStorage(t)
	_, err := adapter.Load(context.Background(), "missing")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("load missing error = %v, want fs.ErrNotExist", err)
	}
}

func newTestCertMagicStorage(t *testing.T) certmagic.Storage {
	t.Helper()
	return newCertMagicStorage(certstore.NewProjection())
}

func assertStorageValue(t *testing.T, storage certmagic.Storage, key, want string) {
	t.Helper()
	loaded, err := storage.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load through adapter: %v", err)
	}
	if string(loaded) != want {
		t.Fatalf("loaded = %q, want %s", loaded, want)
	}
}

func assertStorageKeys(t *testing.T, storage certmagic.Storage, prefix string, recursive bool, want []string) {
	t.Helper()
	keys, err := storage.List(context.Background(), prefix, recursive)
	if err != nil {
		t.Fatalf("list through adapter: %v", err)
	}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for index := range want {
		if keys[index] != want[index] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
}

func assertTerminalStat(t *testing.T, storage certmagic.Storage, key string, wantSize int64) {
	t.Helper()
	info, err := storage.Stat(context.Background(), key)
	if err != nil {
		t.Fatalf("stat through adapter: %v", err)
	}
	if !info.IsTerminal || info.Size != wantSize {
		t.Fatalf("stat = %+v, want terminal size %d", info, wantSize)
	}
}

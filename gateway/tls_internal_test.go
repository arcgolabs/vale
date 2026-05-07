package gateway

import (
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
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

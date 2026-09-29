package runtime

import (
	"net/http"
	"testing"
)

func TestBuildCatalogIncludesHandlerEndpoint(t *testing.T) {
	t.Parallel()

	endpoint, err := NewHandlerEndpoint("static", 1, http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := BuildCatalog(NewSnapshot().AddService(NewService("static", "round_robin", endpoint)))
	if err != nil {
		t.Fatal(err)
	}
	txn := catalog.db.Txn(false)
	defer txn.Abort()
	raw, err := txn.First(catalogTableEndpoint, "id", "static/000000")
	if err != nil {
		t.Fatal(err)
	}
	record, ok := raw.(EndpointRecord)
	if !ok {
		t.Fatalf("catalog endpoint = %#v, want EndpointRecord", raw)
	}
	if record.Kind != EndpointKindHandler || record.Name != "static" || record.URL != "" {
		t.Fatalf("catalog endpoint = %#v, want handler kind, static name, and empty URL", record)
	}
}

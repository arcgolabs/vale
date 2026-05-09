package certstore_test

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"

	"github.com/arcgolabs/vale/certstore"
)

func TestProjectionStoresDefensiveCopies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := certstore.NewProjection()
	value := []byte("cert")
	if err := store.Store(ctx, "certificates/example/cert.pem", value); err != nil {
		t.Fatalf("store certificate: %v", err)
	}
	value[0] = 'X'

	loaded, err := store.Load(ctx, "certificates/example/cert.pem")
	if err != nil {
		t.Fatalf("load certificate: %v", err)
	}
	if string(loaded) != "cert" {
		t.Fatalf("loaded certificate = %q, want cert", loaded)
	}
	loaded[0] = 'Y'
	loadedAgain, err := store.Load(ctx, "certificates/example/cert.pem")
	if err != nil {
		t.Fatalf("load certificate again: %v", err)
	}
	if string(loadedAgain) != "cert" {
		t.Fatalf("loaded certificate after mutation = %q, want cert", loadedAgain)
	}
}

func TestProjectionListUsesFileStorageSemantics(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := certstore.NewProjection(
		certstore.Object{Key: "certificates/local/example.com/example.com.crt", Value: []byte("cert")},
		certstore.Object{Key: "certificates/local/example.com/example.com.key", Value: []byte("key")},
		certstore.Object{Key: "ocsp/example", Value: []byte("staple")},
	)

	topLevel, err := store.List(ctx, "certificates", false)
	if err != nil {
		t.Fatalf("list top-level certificates: %v", err)
	}
	assertStringList(t, topLevel.Values(), []string{"certificates/local"})

	recursive, err := store.List(ctx, "certificates", true)
	if err != nil {
		t.Fatalf("list recursive certificates: %v", err)
	}
	assertStringList(t, recursive.Values(), []string{
		"certificates/local",
		"certificates/local/example.com",
		"certificates/local/example.com/example.com.crt",
		"certificates/local/example.com/example.com.key",
	})

	_, err = store.List(ctx, "missing", false)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("list missing error = %v, want fs.ErrNotExist", err)
	}
}

func TestProjectionStatAndDeleteDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := certstore.NewProjection(
		certstore.Object{Key: "certificates/local/a.crt", Value: []byte("cert")},
		certstore.Object{Key: "certificates/local/a.key", Value: []byte("key")},
	)

	info, err := store.Stat(ctx, "certificates/local")
	if err != nil {
		t.Fatalf("stat directory: %v", err)
	}
	if info.IsTerminal {
		t.Fatalf("directory stat IsTerminal = true, want false")
	}

	if err := store.Delete(ctx, "certificates/local"); err != nil {
		t.Fatalf("delete directory: %v", err)
	}
	if store.Exists(ctx, "certificates/local/a.crt") {
		t.Fatalf("deleted certificate still exists")
	}
}

func TestProjectionLocalLockerSerializesByName(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := certstore.NewProjection()
	if err := store.Lock(ctx, "acme/example"); err != nil {
		t.Fatalf("lock: %v", err)
	}

	waiting := make(chan struct{})
	acquired := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		close(waiting)
		if err := store.Lock(ctx, "acme/example"); err != nil {
			t.Errorf("second lock: %v", err)
			return
		}
		close(acquired)
		if err := store.Unlock(ctx, "acme/example"); err != nil {
			t.Errorf("second unlock: %v", err)
		}
	})
	<-waiting
	select {
	case <-acquired:
		t.Fatalf("second lock acquired before first unlock")
	default:
	}
	if err := store.Unlock(ctx, "acme/example"); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	<-acquired
	wg.Wait()
}

func assertStringList(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("list = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("list = %v, want %v", got, want)
		}
	}
}

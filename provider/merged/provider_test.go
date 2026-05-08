package merged_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider/memoryconfig"
	"github.com/arcgolabs/vale/provider/merged"
	"github.com/arcgolabs/vale/runtime"
)

func TestWatchSkipsUnchangedFingerprint(t *testing.T) {
	t.Parallel()

	source := newMemorySource(t, config.Default())
	provider := newLoadedProvider(t, source, 10*time.Millisecond)
	reloads, closer := watchProvider(t, provider, 2)
	defer closeProvider(t, closer)

	updateSource(t, source, config.Default())
	assertNoReload(t, reloads, 50*time.Millisecond, "unchanged config triggered reload")

	updateSource(t, source, configWithAdmin(":19091"))
	assertReload(t, reloads, 200*time.Millisecond)
}

func TestWatchCoalescesRapidChanges(t *testing.T) {
	t.Parallel()

	source := newMemorySource(t, config.Default())
	provider := newLoadedProvider(t, source, 20*time.Millisecond)
	reloads, closer := watchProvider(t, provider, 4)
	defer closeProvider(t, closer)

	updateSource(t, source, configWithAdmin(":19091"))
	updateSource(t, source, configWithAdmin(":19092"))

	snapshot := assertReload(t, reloads, 250*time.Millisecond)
	if snapshot.AdminAddress != ":19092" {
		t.Fatalf("reloaded admin address = %q, want latest :19092", snapshot.AdminAddress)
	}
	assertNoReload(t, reloads, 80*time.Millisecond, "unexpected second reload")
}

func TestWatchCloseStopsReload(t *testing.T) {
	t.Parallel()

	source := newMemorySource(t, config.Default())
	provider := newLoadedProvider(t, source, 10*time.Millisecond)
	reloads, closer := watchProvider(t, provider, 1)
	closeProvider(t, closer)

	updateSource(t, source, configWithAdmin(":19091"))
	assertNoReload(t, reloads, 50*time.Millisecond, "closed watcher received reload")
}

func newMemorySource(t *testing.T, cfg *config.Config) *memoryconfig.Provider {
	t.Helper()
	source, err := memoryconfig.New("memory", cfg)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func newLoadedProvider(t *testing.T, source *memoryconfig.Provider, debounce time.Duration) *merged.Provider {
	t.Helper()
	provider := merged.New(nil, merged.Source{Name: "memory", Provider: source})
	provider.SetReloadDebounce(debounce)
	if _, err := provider.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	return provider
}

func watchProvider(t *testing.T, provider *merged.Provider, buffer int) (chan *runtime.CompiledSnapshot, io.Closer) {
	t.Helper()
	reloads := make(chan *runtime.CompiledSnapshot, buffer)
	closer, err := provider.Watch(context.Background(), func(snapshot *runtime.CompiledSnapshot) {
		reloads <- snapshot
	}, func(err error) {
		t.Errorf("watch error: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	return reloads, closer
}

func closeProvider(t *testing.T, closer io.Closer) {
	t.Helper()
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
}

func updateSource(t *testing.T, source *memoryconfig.Provider, cfg *config.Config) {
	t.Helper()
	if err := source.Update(cfg); err != nil {
		t.Fatal(err)
	}
}

func assertReload(
	t *testing.T,
	reloads <-chan *runtime.CompiledSnapshot,
	timeout time.Duration,
) *runtime.CompiledSnapshot {
	t.Helper()
	select {
	case snapshot := <-reloads:
		return snapshot
	case <-time.After(timeout):
		t.Fatal("changed config did not trigger reload")
	}
	return nil
}

func assertNoReload(
	t *testing.T,
	reloads <-chan *runtime.CompiledSnapshot,
	timeout time.Duration,
	message string,
) {
	t.Helper()
	select {
	case snapshot := <-reloads:
		t.Fatalf("%s: %#v", message, snapshot)
	case <-time.After(timeout):
	}
}

func configWithAdmin(address string) *config.Config {
	cfg := config.Default()
	cfg.Admin.Address = address
	return cfg
}

package merged_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vale/config"
	providerevents "github.com/arcgolabs/vale/provider"
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

func TestWatchPublishesDebounceMetricsEvent(t *testing.T) {
	t.Parallel()

	source := newMemorySource(t, config.Default())
	bus := eventx.New()
	debounce := 20 * time.Millisecond
	provider := newLoadedProviderWithBus(t, source, debounce, bus)
	reloads, debounceEvents, closer := watchProviderWithMetrics(t, provider, bus, 1)
	defer closeProvider(t, closer)

	updateSource(t, source, configWithAdmin(":19091"))
	updateSource(t, source, configWithAdmin(":19092"))

	assertReload(t, reloads, 250*time.Millisecond)
	debounceEvent := assertDebounceEvent(t, debounceEvents, 250*time.Millisecond)
	if debounceEvent.SourceCount != 1 {
		t.Fatalf("debounce SourceCount = %d, want 1", debounceEvent.SourceCount)
	}
	if debounceEvent.Source != "memory" {
		t.Fatalf("debounce Source = %q, want memory", debounceEvent.Source)
	}
	if debounceEvent.DebounceTime < debounce {
		t.Fatalf("debounce Delay = %s, want at least %s", debounceEvent.DebounceTime, debounce)
	}
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

func newLoadedProviderWithBus(
	t *testing.T,
	source *memoryconfig.Provider,
	debounce time.Duration,
	bus eventx.BusRuntime,
) *merged.Provider {
	t.Helper()
	provider := merged.New(bus, merged.Source{Name: "memory", Provider: source})
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

func watchProviderWithMetrics(
	t *testing.T,
	provider *merged.Provider,
	bus eventx.BusRuntime,
	buffer int,
) (chan *runtime.CompiledSnapshot, chan providerevents.ConfigSourceDebouncedEvent, io.Closer) {
	t.Helper()
	reloads := make(chan *runtime.CompiledSnapshot, buffer)
	if bus == nil {
		t.Fatal("watchProviderWithEvents requires a non-nil bus")
	}
	debounceEvents := make(chan providerevents.ConfigSourceDebouncedEvent, buffer)
	debouncedUnsub, err := eventx.Subscribe[providerevents.ConfigSourceDebouncedEvent](bus, func(_ context.Context, event providerevents.ConfigSourceDebouncedEvent) error {
		select {
		case debounceEvents <- event:
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		debouncedUnsub()
	})

	closer, err := provider.Watch(context.Background(), func(snapshot *runtime.CompiledSnapshot) {
		reloads <- snapshot
	}, func(err error) {
		t.Errorf("watch error: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	return reloads, debounceEvents, closer
}

func assertDebounceEvent(
	t *testing.T,
	events <-chan providerevents.ConfigSourceDebouncedEvent,
	timeout time.Duration,
) providerevents.ConfigSourceDebouncedEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(timeout):
		t.Fatal("did not receive config source debounced event")
	}
	return providerevents.ConfigSourceDebouncedEvent{}
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

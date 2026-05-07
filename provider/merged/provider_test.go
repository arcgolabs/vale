package merged

import (
	"context"
	"testing"
	"time"

	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/provider/memoryconfig"
	"github.com/arcgolabs/vela/runtime"
)

func TestWatchSkipsUnchangedFingerprint(t *testing.T) {
	t.Parallel()

	initial := config.Default()
	source, err := memoryconfig.New("memory", initial)
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, Source{Name: "memory", Provider: source})
	p.reloadDebounce = 10 * time.Millisecond
	if _, err := p.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	reloads := make(chan *runtime.CompiledSnapshot, 2)
	closer, err := p.Watch(context.Background(), func(snapshot *runtime.CompiledSnapshot) {
		reloads <- snapshot
	}, func(err error) {
		t.Errorf("watch error: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	if err := source.Update(config.Default()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reloads:
		t.Fatal("unchanged config triggered reload")
	case <-time.After(50 * time.Millisecond):
	}

	changed := config.Default()
	changed.Admin.Address = ":19091"
	if err := source.Update(changed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reloads:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("changed config did not trigger reload")
	}
}

func TestWatchCoalescesRapidChanges(t *testing.T) {
	t.Parallel()

	source, err := memoryconfig.New("memory", config.Default())
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, Source{Name: "memory", Provider: source})
	p.reloadDebounce = 20 * time.Millisecond
	if _, err := p.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	reloads := make(chan *runtime.CompiledSnapshot, 4)
	closer, err := p.Watch(context.Background(), func(snapshot *runtime.CompiledSnapshot) {
		reloads <- snapshot
	}, func(err error) {
		t.Errorf("watch error: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()

	first := config.Default()
	first.Admin.Address = ":19091"
	if err := source.Update(first); err != nil {
		t.Fatal(err)
	}
	second := config.Default()
	second.Admin.Address = ":19092"
	if err := source.Update(second); err != nil {
		t.Fatal(err)
	}

	select {
	case snapshot := <-reloads:
		if snapshot.AdminAddress != ":19092" {
			t.Fatalf("reloaded admin address = %q, want latest :19092", snapshot.AdminAddress)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("coalesced reload did not fire")
	}

	select {
	case snapshot := <-reloads:
		t.Fatalf("unexpected second reload: %#v", snapshot)
	case <-time.After(80 * time.Millisecond):
	}
}

func TestWatchCloseStopsReload(t *testing.T) {
	t.Parallel()

	source, err := memoryconfig.New("memory", config.Default())
	if err != nil {
		t.Fatal(err)
	}
	p := New(nil, Source{Name: "memory", Provider: source})
	p.reloadDebounce = 10 * time.Millisecond
	if _, err := p.Load(context.Background()); err != nil {
		t.Fatal(err)
	}

	reloads := make(chan *runtime.CompiledSnapshot, 1)
	closer, err := p.Watch(context.Background(), func(snapshot *runtime.CompiledSnapshot) {
		reloads <- snapshot
	}, func(err error) {
		t.Errorf("watch error: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}

	changed := config.Default()
	changed.Admin.Address = ":19091"
	if err := source.Update(changed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reloads:
		t.Fatal("closed watcher received reload")
	case <-time.After(50 * time.Millisecond):
	}
}

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

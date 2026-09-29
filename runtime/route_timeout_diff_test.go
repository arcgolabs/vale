package runtime_test

import (
	"testing"
	"time"

	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestDiffSnapshotsDetectsRouteWriteTimeoutChange(t *testing.T) {
	t.Parallel()

	current := valeruntime.NewSnapshot().AddRoute(valeruntime.NewRoute("events", "web", nil))
	next := valeruntime.NewSnapshot().AddRoute(
		valeruntime.NewRoute("events", "web", nil).WithWriteTimeout(0 * time.Second),
	)
	diff := valeruntime.DiffSnapshots(current, next)
	if diff.Routes.Changed.Len() != 1 {
		t.Fatalf("changed routes = %v, want events", diff.Routes.Changed.Values())
	}
	name, _ := diff.Routes.Changed.Get(0)
	if name != "events" {
		t.Fatalf("changed route = %q, want events", name)
	}
}

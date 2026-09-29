package genericx_test

import (
	"testing"

	"github.com/arcgolabs/vale/internal/genericx"
)

func TestIsNilDetectsTypedNilAfterInterfaceErasure(t *testing.T) {
	t.Parallel()

	var pointer *int
	var erased any = pointer
	if !genericx.IsNil(erased) {
		t.Fatal("typed nil pointer stored in interface was not detected")
	}
	if genericx.IsNil(42) {
		t.Fatal("non-nil value was reported as nil")
	}
}

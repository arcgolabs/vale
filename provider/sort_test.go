package provider

import (
	"reflect"
	"testing"
)

func TestSortedStrings(t *testing.T) {
	input := []string{"b", "a", "c"}

	got := SortedStrings(input)

	if !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("unexpected sorted values: %#v", got)
	}
	if !reflect.DeepEqual(input, []string{"b", "a", "c"}) {
		t.Fatalf("input was mutated: %#v", input)
	}
}

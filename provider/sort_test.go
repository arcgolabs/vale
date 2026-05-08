package provider_test

import (
	"reflect"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/provider"
)

func TestSortedStrings(t *testing.T) {
	input := collectionlist.NewList("b", "a", "c")

	got := provider.SortedStrings(input)

	if !reflect.DeepEqual(got.Values(), []string{"a", "b", "c"}) {
		t.Fatalf("unexpected sorted values: %#v", got)
	}
	if !reflect.DeepEqual(input.Values(), []string{"b", "a", "c"}) {
		t.Fatalf("input was mutated: %#v", input)
	}
}

package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

// SortedStrings returns a sorted copy of values.
func SortedStrings(values *collectionlist.List[string]) *collectionlist.List[string] {
	return values.Clone().Sort(strings.Compare)
}

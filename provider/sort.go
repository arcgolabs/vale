package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

// SortedStrings returns a sorted copy of values.
func SortedStrings(values []string) []string {
	return collectionlist.NewList[string](values...).Sort(strings.Compare).Values()
}

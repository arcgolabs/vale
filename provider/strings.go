package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func SplitCSV(value string) []string {
	return collectionlist.FilterMapList(collectionlist.NewList(strings.Split(value, ",")...), func(_ int, part string) (string, bool) {
		trimmed := strings.TrimSpace(part)
		return trimmed, trimmed != ""
	}).Values()
}

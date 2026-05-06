package provider

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func SplitCSV(value string) []string {
	parts := collectionlist.NewList(strings.Split(value, ",")...)
	return collectionlist.FilterMapList(parts, func(_ int, part string) (string, bool) {
		trimmed := strings.TrimSpace(part)
		return trimmed, trimmed != ""
	}).Values()
}

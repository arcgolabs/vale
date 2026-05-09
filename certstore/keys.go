package certstore

import (
	"path"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func cleanKey(key string) string {
	key = strings.ReplaceAll(strings.TrimSpace(key), "\\", "/")
	if key == "" {
		return ""
	}
	key = path.Clean("/" + key)
	key = strings.TrimPrefix(key, "/")
	if key == "." {
		return ""
	}
	return strings.Trim(key, "/")
}

func keyPrefix(key string) string {
	key = cleanKey(key)
	if key == "" {
		return ""
	}
	return key + "/"
}

func listedKeys(prefix, key string, recursive bool) []string {
	key = cleanKey(key)
	if key == "" {
		return nil
	}
	relative, ok := relativeStorageKey(prefix, key)
	if !ok || relative == "" {
		return nil
	}
	parts := strings.Split(relative, "/")
	if !recursive {
		return []string{joinStorageKey(prefix, parts[0])}
	}
	keys := collectionlist.NewListWithCapacity[string](len(parts))
	for index := range parts {
		keys.Add(joinStorageKey(prefix, strings.Join(parts[:index+1], "/")))
	}
	return keys.Values()
}

func relativeStorageKey(prefix, key string) (string, bool) {
	if prefix == "" {
		return key, true
	}
	if key == prefix {
		return "", true
	}
	base := keyPrefix(prefix)
	if !strings.HasPrefix(key, base) {
		return "", false
	}
	return strings.TrimPrefix(key, base), true
}

func joinStorageKey(prefix, suffix string) string {
	prefix = cleanKey(prefix)
	suffix = cleanKey(suffix)
	if prefix == "" {
		return suffix
	}
	if suffix == "" {
		return prefix
	}
	return prefix + "/" + suffix
}

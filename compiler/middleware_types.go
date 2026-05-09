package compiler

import (
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/vale/runtime"
)

var supportedMiddlewareTypes = collectionset.NewSet(runtime.MiddlewareTypeBuiltin)

func middlewareTypeSet(extraTypes *collectionlist.List[string]) *collectionset.Set[string] {
	supportedTypes := supportedMiddlewareTypes.Clone()
	if extraTypes == nil {
		return supportedTypes
	}
	extraTypes.Range(func(_ int, middlewareType string) bool {
		supportedTypes.Add(normalizeMiddlewareType(middlewareType))
		return true
	})
	return supportedTypes
}

func normalizeMiddlewareType(middlewareType string) string {
	middlewareType = strings.ToLower(strings.TrimSpace(middlewareType))
	switch middlewareType {
	case "",
		"builtin",
		"add_prefix",
		"basic_auth",
		"basicauth",
		"buffering",
		"chain",
		"compress",
		"headers",
		"ip_allow_list",
		"ipallowlist",
		"ipwhitelist",
		"redirect_regex",
		"redirect_scheme",
		"replace_path",
		"replace_path_regex",
		"strip_prefix":
		return runtime.MiddlewareTypeBuiltin
	default:
		return middlewareType
	}
}

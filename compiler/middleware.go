package compiler

import (
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/vela/config"
	"github.com/arcgolabs/vela/runtime"
)

var supportedMiddlewareTypes = collectionset.NewSet(runtime.MiddlewareTypeBuiltin)

func compileMiddlewares(middlewares []config.Middleware) (*mapping.Map[string, runtime.MiddlewareRuntime], error) {
	middlewareMap := mapping.NewMapWithCapacity[string, runtime.MiddlewareRuntime](len(middlewares))
	for index := range middlewares {
		middleware := &middlewares[index]
		compiled, err := compileMiddleware(middleware)
		if err != nil {
			return nil, err
		}
		middlewareMap.Set(middleware.Name, compiled)
	}
	return middlewareMap, nil
}

func compileMiddleware(middleware *config.Middleware) (runtime.MiddlewareRuntime, error) {
	middlewareType := normalizeMiddlewareType(middleware.Type)
	if !supportedMiddlewareTypes.Contains(middlewareType) {
		return runtime.MiddlewareRuntime{}, fmt.Errorf("middleware %q has unsupported type %q", middleware.Name, middleware.Type)
	}
	return runtime.MiddlewareRuntime{
		Name:                   middleware.Name,
		Type:                   middlewareType,
		StripPrefix:            strings.TrimSpace(middleware.StripPrefix),
		StripPrefixes:          cleanStringList(middleware.StripPrefixes),
		AddPrefix:              strings.TrimSpace(middleware.AddPrefix),
		ReplacePath:            strings.TrimSpace(middleware.ReplacePath),
		ReplacePathRegex:       strings.TrimSpace(middleware.ReplacePathRegex),
		ReplacePathReplacement: strings.TrimSpace(middleware.ReplacePathReplacement),
		RedirectScheme:         strings.TrimSpace(middleware.RedirectScheme),
		RedirectPort:           strings.TrimSpace(middleware.RedirectPort),
		RedirectRegex:          strings.TrimSpace(middleware.RedirectRegex),
		RedirectReplacement:    strings.TrimSpace(middleware.RedirectReplacement),
		RedirectPermanent:      middleware.RedirectPermanent,
		RequestHeaders:         normalizeHeaders(middleware.RequestHeaders),
		ResponseHeaders:        normalizeHeaders(middleware.ResponseHeaders),
		MaxBodyBytes:           middleware.MaxBodyBytes,
		Chain:                  cleanStringList(middleware.Chain),
	}, nil
}

func compileRouteMiddlewares(names []string, middlewares *mapping.Map[string, runtime.MiddlewareRuntime]) *collectionlist.List[runtime.MiddlewareRuntime] {
	compiled := collectionlist.NewListWithCapacity[runtime.MiddlewareRuntime](len(names))
	if len(names) == 0 || middlewares == nil {
		return compiled
	}
	visiting := collectionset.NewSet[string]()
	for _, name := range names {
		appendMiddleware(compiled, middlewares, strings.TrimSpace(name), visiting)
	}
	return compiled
}

func appendMiddleware(
	compiled *collectionlist.List[runtime.MiddlewareRuntime],
	middlewares *mapping.Map[string, runtime.MiddlewareRuntime],
	name string,
	visiting *collectionset.Set[string],
) {
	if name == "" || visiting.Contains(name) {
		return
	}
	middleware, ok := middlewares.Get(name)
	if !ok {
		return
	}
	if middleware.Chain == nil || middleware.Chain.IsEmpty() {
		compiled.Add(middleware)
		return
	}
	visiting.Add(name)
	middleware.Chain.Range(func(_ int, child string) bool {
		appendMiddleware(compiled, middlewares, child, visiting)
		return true
	})
	visiting.Remove(name)
}

func normalizeMiddlewareType(middlewareType string) string {
	middlewareType = strings.ToLower(strings.TrimSpace(middlewareType))
	switch middlewareType {
	case "",
		"builtin",
		"add_prefix",
		"buffering",
		"chain",
		"headers",
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

func cleanStringList(values []string) *collectionlist.List[string] {
	cleaned := collectionlist.NewListWithCapacity[string](len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned.Add(trimmed)
		}
	}
	return cleaned
}

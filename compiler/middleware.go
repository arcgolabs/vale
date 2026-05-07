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
		Secure:                 compileSecureMiddleware(middleware.Secure),
		CORS:                   compileCORSMiddleware(middleware.CORS),
		RateLimit:              compileRateLimit(middleware.RateLimit),
		CircuitBreaker:         compileCircuitBreaker(middleware.CircuitBreaker),
	}, nil
}

func compileSecureMiddleware(secure *config.SecureMiddleware) runtime.SecureMiddlewareRuntime {
	if secure == nil {
		return runtime.SecureMiddlewareRuntime{}
	}
	return runtime.SecureMiddlewareRuntime{
		Enabled:                         secure.Enabled || hasSecureMiddlewareOptions(secure),
		AllowedHosts:                    cleanStringList(secure.AllowedHosts),
		AllowedHostsAreRegex:            secure.AllowedHostsAreRegex,
		SSLRedirect:                     secure.SSLRedirect,
		SSLHost:                         strings.TrimSpace(secure.SSLHost),
		SSLTemporaryRedirect:            secure.SSLTemporaryRedirect,
		STSSeconds:                      secure.STSSeconds,
		STSIncludeSubdomains:            secure.STSIncludeSubdomains,
		STSPreload:                      secure.STSPreload,
		FrameDeny:                       secure.FrameDeny,
		ContentTypeNosniff:              secure.ContentTypeNosniff,
		BrowserXSSFilter:                secure.BrowserXSSFilter,
		ContentSecurityPolicy:           strings.TrimSpace(secure.ContentSecurityPolicy),
		ContentSecurityPolicyReportOnly: strings.TrimSpace(secure.ContentSecurityPolicyReportOnly),
		ReferrerPolicy:                  strings.TrimSpace(secure.ReferrerPolicy),
		PermissionsPolicy:               strings.TrimSpace(secure.PermissionsPolicy),
	}
}

func hasSecureMiddlewareOptions(secure *config.SecureMiddleware) bool {
	return hasAnyTrue(collectionlist.NewList(
		len(secure.AllowedHosts) > 0,
		secure.AllowedHostsAreRegex,
		secure.SSLRedirect,
		strings.TrimSpace(secure.SSLHost) != "",
		secure.SSLTemporaryRedirect,
		secure.STSSeconds > 0,
		secure.STSIncludeSubdomains,
		secure.STSPreload,
		secure.FrameDeny,
		secure.ContentTypeNosniff,
		secure.BrowserXSSFilter,
		strings.TrimSpace(secure.ContentSecurityPolicy) != "",
		strings.TrimSpace(secure.ContentSecurityPolicyReportOnly) != "",
		strings.TrimSpace(secure.ReferrerPolicy) != "",
		strings.TrimSpace(secure.PermissionsPolicy) != "",
	))
}

func compileCORSMiddleware(cors *config.CORSMiddleware) runtime.CORSMiddlewareRuntime {
	if cors == nil {
		return runtime.CORSMiddlewareRuntime{}
	}
	return runtime.CORSMiddlewareRuntime{
		Enabled:              cors.Enabled || hasCORSMiddlewareOptions(cors),
		AllowedOrigins:       cleanStringList(cors.AllowedOrigins),
		AllowedMethods:       cleanStringList(cors.AllowedMethods),
		AllowedHeaders:       cleanStringList(cors.AllowedHeaders),
		ExposedHeaders:       cleanStringList(cors.ExposedHeaders),
		MaxAge:               cors.MaxAge,
		AllowCredentials:     cors.AllowCredentials,
		AllowPrivateNetwork:  cors.AllowPrivateNetwork,
		OptionsPassthrough:   cors.OptionsPassthrough,
		OptionsSuccessStatus: cors.OptionsSuccessStatus,
	}
}

func hasCORSMiddlewareOptions(cors *config.CORSMiddleware) bool {
	return hasAnyTrue(collectionlist.NewList(
		len(cors.AllowedOrigins) > 0,
		len(cors.AllowedMethods) > 0,
		len(cors.AllowedHeaders) > 0,
		len(cors.ExposedHeaders) > 0,
		cors.MaxAge != 0,
		cors.AllowCredentials,
		cors.AllowPrivateNetwork,
		cors.OptionsPassthrough,
		cors.OptionsSuccessStatus != 0,
	))
}

func compileRateLimit(rateLimit *config.RateLimit) runtime.RateLimitRuntime {
	if rateLimit == nil {
		return runtime.RateLimitRuntime{}
	}
	burst := rateLimit.Burst
	if burst <= 0 && rateLimit.Rate > 0 {
		burst = 1
	}
	return runtime.RateLimitRuntime{
		Enabled: rateLimit.Enabled || rateLimit.Rate > 0 || burst > 0,
		Rate:    rateLimit.Rate,
		Burst:   burst,
	}
}

func compileCircuitBreaker(circuitBreaker *config.CircuitBreaker) runtime.CircuitBreakerRuntime {
	if circuitBreaker == nil {
		return runtime.CircuitBreakerRuntime{}
	}
	return runtime.CircuitBreakerRuntime{
		Enabled:          circuitBreaker.Enabled || hasCircuitBreakerOptions(circuitBreaker),
		MaxRequests:      circuitBreaker.MaxRequests,
		Interval:         strings.TrimSpace(circuitBreaker.Interval),
		Timeout:          strings.TrimSpace(circuitBreaker.Timeout),
		FailureThreshold: circuitBreaker.FailureThreshold,
	}
}

func hasCircuitBreakerOptions(circuitBreaker *config.CircuitBreaker) bool {
	return hasAnyTrue(collectionlist.NewList(
		circuitBreaker.MaxRequests > 0,
		strings.TrimSpace(circuitBreaker.Interval) != "",
		strings.TrimSpace(circuitBreaker.Timeout) != "",
		circuitBreaker.FailureThreshold > 0,
	))
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

func hasAnyTrue(values *collectionlist.List[bool]) bool {
	matched := false
	values.Range(func(_ int, value bool) bool {
		matched = value
		return !matched
	})
	return matched
}

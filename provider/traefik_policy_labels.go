package provider

import (
	"strings"

	"github.com/arcgolabs/vale/config"
)

func applyTraefikPolicyMiddleware(middleware *config.Middleware, option, value string) bool {
	return applyTraefikAuthMiddleware(middleware, option, value) ||
		applyTraefikCompressMiddleware(middleware, option, value) ||
		applyTraefikIPAllowListMiddleware(middleware, option, value) ||
		applyTraefikRateLimitMiddleware(middleware, option, value) ||
		applyTraefikHeaderPolicyMiddleware(middleware, option, value)
}

func applyTraefikAuthMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "basicauth.realm":
		middleware.BasicAuth = ensureBasicAuth(middleware.BasicAuth)
		middleware.BasicAuth.Realm = strings.TrimSpace(value)
	case "basicauth.users":
		middleware.BasicAuth = ensureBasicAuth(middleware.BasicAuth)
		middleware.BasicAuth.Users = parseTraefikBasicAuthUsers(value)
	default:
		return false
	}
	return true
}

func applyTraefikCompressMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "compress":
		middleware.Compress = ensureCompress(middleware.Compress)
		middleware.Compress.Enabled = parseTraefikBool(value)
	case "compress.minresponsebodybytes":
		middleware.Compress = ensureCompress(middleware.Compress)
		middleware.Compress.MinBytes = parseTraefikInt(value)
	default:
		return false
	}
	return true
}

func applyTraefikIPAllowListMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "ipallowlist.sourcerange", "ipwhitelist.sourcerange":
		middleware.IPAllowList = ensureIPAllowList(middleware.IPAllowList)
		middleware.IPAllowList.SourceRange = SplitCSV(value).Values()
	case "ipallowlist.trustforwardheader", "ipwhitelist.trustforwardheader":
		middleware.IPAllowList = ensureIPAllowList(middleware.IPAllowList)
		middleware.IPAllowList.TrustForwardHeader = parseTraefikBool(value)
	default:
		return false
	}
	return true
}

func applyTraefikRateLimitMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "ratelimit.average":
		middleware.RateLimit = ensureRateLimit(middleware.RateLimit)
		middleware.RateLimit.Rate = float64(parseTraefikInt(value))
	case "ratelimit.burst":
		middleware.RateLimit = ensureRateLimit(middleware.RateLimit)
		middleware.RateLimit.Burst = parseTraefikInt(value)
	default:
		return false
	}
	return true
}

func applyTraefikHeaderPolicyMiddleware(middleware *config.Middleware, option, value string) bool {
	if header, ok := strings.CutPrefix(option, "headers.customrequestheaders."); ok {
		setHeader(middleware.RequestHeaders, header, value)
		return true
	}
	if header, ok := strings.CutPrefix(option, "headers.customresponseheaders."); ok {
		setHeader(middleware.ResponseHeaders, header, value)
		return true
	}
	return applyTraefikCORSMiddleware(middleware, option, value)
}

func applyTraefikCORSMiddleware(middleware *config.Middleware, option, value string) bool {
	switch option {
	case "headers.accesscontrolalloworiginlist":
		middleware.CORS = ensureCORS(middleware.CORS)
		middleware.CORS.AllowedOrigins = SplitCSV(value).Values()
	case "headers.accesscontrolallowmethods":
		middleware.CORS = ensureCORS(middleware.CORS)
		middleware.CORS.AllowedMethods = SplitCSV(value).Values()
	case "headers.accesscontrolallowheaders":
		middleware.CORS = ensureCORS(middleware.CORS)
		middleware.CORS.AllowedHeaders = SplitCSV(value).Values()
	case "headers.accesscontrolexposeheaders":
		middleware.CORS = ensureCORS(middleware.CORS)
		middleware.CORS.ExposedHeaders = SplitCSV(value).Values()
	case "headers.accesscontrolallowcredentials":
		middleware.CORS = ensureCORS(middleware.CORS)
		middleware.CORS.AllowCredentials = parseTraefikBool(value)
	case "headers.accesscontrolmaxage":
		middleware.CORS = ensureCORS(middleware.CORS)
		middleware.CORS.MaxAge = parseTraefikInt(value)
	default:
		return false
	}
	return true
}

func ensureBasicAuth(basicAuth *config.BasicAuth) *config.BasicAuth {
	if basicAuth == nil {
		return &config.BasicAuth{Enabled: true, Users: map[string]string{}}
	}
	basicAuth.Enabled = true
	if basicAuth.Users == nil {
		basicAuth.Users = map[string]string{}
	}
	return basicAuth
}

func ensureCompress(compress *config.Compress) *config.Compress {
	if compress == nil {
		return &config.Compress{Enabled: true}
	}
	compress.Enabled = true
	return compress
}

func ensureIPAllowList(ipAllowList *config.IPAllowList) *config.IPAllowList {
	if ipAllowList == nil {
		return &config.IPAllowList{Enabled: true}
	}
	ipAllowList.Enabled = true
	return ipAllowList
}

func ensureRateLimit(rateLimit *config.RateLimit) *config.RateLimit {
	if rateLimit == nil {
		return &config.RateLimit{Enabled: true}
	}
	rateLimit.Enabled = true
	return rateLimit
}

func ensureCORS(cors *config.CORSMiddleware) *config.CORSMiddleware {
	if cors == nil {
		return &config.CORSMiddleware{Enabled: true}
	}
	cors.Enabled = true
	return cors
}

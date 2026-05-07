package runtime

import (
	"errors"
	"net/http"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/rs/cors"
	"github.com/samber/mo"
	"github.com/samber/oops"
	"github.com/sony/gobreaker"
	"github.com/unrolled/secure"
	"golang.org/x/time/rate"
)

var errCircuitBreakerFailure = errors.New("middleware circuit breaker counted server failure")

func wrapSecureMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.Secure.Enabled {
		return next
	}
	secureHeadersDefault := secureHeaderDefaultsNeeded(middleware.Secure)
	return secure.New(secure.Options{
		AllowedHosts:                    stringListValues(middleware.Secure.AllowedHosts),
		AllowedHostsAreRegex:            middleware.Secure.AllowedHostsAreRegex,
		SSLRedirect:                     middleware.Secure.SSLRedirect,
		SSLHost:                         middleware.Secure.SSLHost,
		SSLTemporaryRedirect:            middleware.Secure.SSLTemporaryRedirect,
		SSLProxyHeaders:                 map[string]string{"X-Forwarded-Proto": "https"},
		STSSeconds:                      middleware.Secure.STSSeconds,
		STSIncludeSubdomains:            middleware.Secure.STSIncludeSubdomains,
		STSPreload:                      middleware.Secure.STSPreload,
		FrameDeny:                       middleware.Secure.FrameDeny || secureHeadersDefault,
		ContentTypeNosniff:              middleware.Secure.ContentTypeNosniff || secureHeadersDefault,
		BrowserXssFilter:                middleware.Secure.BrowserXSSFilter || secureHeadersDefault,
		ContentSecurityPolicy:           middleware.Secure.ContentSecurityPolicy,
		ContentSecurityPolicyReportOnly: middleware.Secure.ContentSecurityPolicyReportOnly,
		ReferrerPolicy:                  defaultString(middleware.Secure.ReferrerPolicy, "no-referrer"),
		PermissionsPolicy:               middleware.Secure.PermissionsPolicy,
	}).Handler(next)
}

func secureHeaderDefaultsNeeded(secureRuntime SecureMiddlewareRuntime) bool {
	return !secureRuntime.FrameDeny &&
		!secureRuntime.ContentTypeNosniff &&
		!secureRuntime.BrowserXSSFilter &&
		secureRuntime.ContentSecurityPolicy == "" &&
		secureRuntime.ContentSecurityPolicyReportOnly == "" &&
		secureRuntime.ReferrerPolicy == "" &&
		secureRuntime.PermissionsPolicy == ""
}

func wrapCORSMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.CORS.Enabled {
		return next
	}
	return cors.New(cors.Options{
		AllowedOrigins:       stringListValues(middleware.CORS.AllowedOrigins),
		AllowedMethods:       stringListValues(middleware.CORS.AllowedMethods),
		AllowedHeaders:       stringListValues(middleware.CORS.AllowedHeaders),
		ExposedHeaders:       stringListValues(middleware.CORS.ExposedHeaders),
		MaxAge:               middleware.CORS.MaxAge,
		AllowCredentials:     middleware.CORS.AllowCredentials,
		AllowPrivateNetwork:  middleware.CORS.AllowPrivateNetwork,
		OptionsPassthrough:   middleware.CORS.OptionsPassthrough,
		OptionsSuccessStatus: middleware.CORS.OptionsSuccessStatus,
	}).Handler(next)
}

func wrapRateLimitMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.RateLimit.Enabled || middleware.RateLimit.Rate <= 0 {
		return next
	}
	limiter := rate.NewLimiter(rate.Limit(middleware.RateLimit.Rate), maxInt(middleware.RateLimit.Burst, 1))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func wrapCircuitBreakerMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.CircuitBreaker.Enabled {
		return next
	}
	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        middleware.Name,
		MaxRequests: middleware.CircuitBreaker.MaxRequests,
		Interval:    parseDuration(middleware.CircuitBreaker.Interval),
		Timeout:     parseDuration(middleware.CircuitBreaker.Timeout),
		ReadyToTrip: circuitBreakerReadyToTrip(middleware.CircuitBreaker.FailureThreshold),
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &circuitStatusRecorder{ResponseWriter: w, status: http.StatusOK}
		_, err := breaker.Execute(func() (any, error) {
			next.ServeHTTP(recorder, r)
			if recorder.status >= http.StatusInternalServerError {
				return nil, errCircuitBreakerFailure
			}
			return nil, nil
		})
		if err != nil && !recorder.wrote {
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
	})
}

func circuitBreakerReadyToTrip(threshold uint32) func(gobreaker.Counts) bool {
	if threshold == 0 {
		return nil
	}
	return func(counts gobreaker.Counts) bool {
		return counts.ConsecutiveFailures >= threshold
	}
}

type circuitStatusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *circuitStatusRecorder) WriteHeader(statusCode int) {
	if r.wrote {
		return
	}
	r.status = statusCode
	r.wrote = true
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *circuitStatusRecorder) Write(body []byte) (int, error) {
	if !r.wrote {
		r.WriteHeader(http.StatusOK)
	}
	written, err := r.ResponseWriter.Write(body)
	if err != nil {
		return written, oops.
			In("runtime").
			With("middleware", "circuit_breaker").
			Wrapf(err, "write response")
	}
	return written, nil
}

func stringListValues(values *collectionlist.List[string]) []string {
	if values == nil || values.IsEmpty() {
		return nil
	}
	return values.Values()
}

func parseDuration(value string) time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	return mo.TupleToOption(duration, err == nil).OrElse(0)
}

func maxInt(value, fallback int) int {
	return mo.TupleToOption(value, value > 0).OrElse(fallback)
}

func defaultString(value, fallback string) string {
	return mo.EmptyableToOption(strings.TrimSpace(value)).OrElse(fallback)
}

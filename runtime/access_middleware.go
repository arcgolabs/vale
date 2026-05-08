package runtime

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func wrapBasicAuthMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.BasicAuth.Enabled || middleware.BasicAuth.Users == nil || middleware.BasicAuth.Users.IsEmpty() {
		return next
	}
	realm := defaultString(middleware.BasicAuth.Realm, "Vale")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		expected, exists := middleware.BasicAuth.Users.Get(username)
		if !ok || !exists || subtle.ConstantTimeCompare([]byte(password), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func wrapIPAllowListMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	if !middleware.IPAllowList.Enabled || middleware.IPAllowList.SourceRange == nil || middleware.IPAllowList.SourceRange.IsEmpty() {
		return next
	}
	ranges := compileIPRanges(middleware.IPAllowList.SourceRange)
	if ranges.IsEmpty() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := requestClientIP(r, middleware.IPAllowList.TrustForwardHeader)
		if ip == nil || !ipAllowed(ip, ranges) {
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ipRange struct {
	ip  net.IP
	net *net.IPNet
}

func compileIPRanges(sources *collectionlist.List[string]) *collectionlist.List[ipRange] {
	return collectionlist.FilterMapList(sources, func(_ int, source string) (ipRange, bool) {
		source = strings.TrimSpace(source)
		if source == "" {
			return ipRange{}, false
		}
		if ip := net.ParseIP(source); ip != nil {
			return ipRange{ip: ip}, true
		}
		_, network, err := net.ParseCIDR(source)
		return ipRange{net: network}, err == nil
	})
}

func requestClientIP(r *http.Request, trustForwardHeader bool) net.IP {
	if ip := forwardedClientIP(r, trustForwardHeader); ip != nil {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func forwardedClientIP(r *http.Request, trustForwardHeader bool) net.IP {
	if !trustForwardHeader {
		return nil
	}
	if forwarded := firstForwardedFor(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		if ip := net.ParseIP(forwarded); ip != nil {
			return ip
		}
	}
	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	return net.ParseIP(realIP)
}

func firstForwardedFor(value string) string {
	first, _, _ := strings.Cut(value, ",")
	return strings.TrimSpace(first)
}

func ipAllowed(ip net.IP, ranges *collectionlist.List[ipRange]) bool {
	return ranges.AnyMatch(func(_ int, candidate ipRange) bool {
		if candidate.net != nil {
			return candidate.net.Contains(ip)
		}
		return candidate.ip != nil && candidate.ip.Equal(ip)
	})
}

package runtime

import (
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

func redirectTarget(r *http.Request, middleware MiddlewareRuntime, redirectRegex *regexp.Regexp) (string, bool) {
	if middleware.RedirectScheme != "" {
		return schemeRedirectTarget(r, middleware), true
	}
	if redirectRegex == nil {
		return "", false
	}
	current := requestAbsoluteURL(r)
	if !redirectRegex.MatchString(current) {
		return "", false
	}
	return redirectRegex.ReplaceAllString(current, middleware.RedirectReplacement), true
}

func writeRedirect(w http.ResponseWriter, target string, status int) {
	location, ok := safeRedirectLocation(target)
	if !ok {
		http.Error(w, "invalid redirect target", http.StatusBadRequest)
		return
	}
	w.Header().Set("Location", location)
	w.WriteHeader(status)
}

func safeRedirectLocation(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.ContainsAny(target, "\r\n") {
		return "", false
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return "", false
	}
	if parsed.IsAbs() {
		if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return "", false
		}
		return parsed.String(), true
	}
	if strings.HasPrefix(target, "//") || !strings.HasPrefix(target, "/") {
		return "", false
	}
	return parsed.String(), true
}

func schemeRedirectTarget(r *http.Request, middleware MiddlewareRuntime) string {
	target := *r.URL
	target.Scheme = middleware.RedirectScheme
	target.Host = redirectHost(r.Host, middleware.RedirectPort)
	return target.String()
}

func redirectHost(host, port string) string {
	if strings.TrimSpace(port) == "" {
		return host
	}
	name, _, err := net.SplitHostPort(host)
	if err != nil {
		name = host
	}
	return net.JoinHostPort(name, port)
}

func requestAbsoluteURL(r *http.Request) string {
	target := *r.URL
	if target.Scheme == "" {
		target.Scheme = requestScheme(r)
	}
	if target.Host == "" {
		target.Host = r.Host
	}
	return target.String()
}

func requestScheme(r *http.Request) string {
	if scheme := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); scheme != "" {
		return scheme
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

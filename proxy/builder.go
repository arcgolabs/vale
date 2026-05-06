package proxy

import (
	"net/http"
	"net/url"
	"strings"

	oxyforward "github.com/vulcand/oxy/v2/forward"
)

const OxyEngineName = "oxy"

type Engine interface {
	Name() string
	Build(*url.URL) http.Handler
}

type OxyEngine struct{}

var DefaultEngine Engine = OxyEngine{}

func (OxyEngine) Name() string {
	return OxyEngineName
}

func (OxyEngine) Build(target *url.URL) http.Handler {
	fwd := oxyforward.New(true)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyReq := r.Clone(r.Context())
		proxyReq.URL = rewriteTargetURL(target, r.URL)
		proxyReq.Host = target.Host
		fwd.ServeHTTP(w, proxyReq)
	})
}

func Build(target *url.URL) http.Handler {
	return DefaultEngine.Build(target)
}

func rewriteTargetURL(target *url.URL, requestURL *url.URL) *url.URL {
	rewritten := *target
	rewritten.Path = joinURLPath(target.Path, requestURL.Path)
	rewritten.RawQuery = joinRawQuery(target.RawQuery, requestURL.RawQuery)
	return &rewritten
}

func joinURLPath(base string, requestPath string) string {
	if base == "" {
		if requestPath == "" {
			return "/"
		}
		return requestPath
	}
	if requestPath == "" {
		return base
	}

	baseSlash := strings.HasSuffix(base, "/")
	requestSlash := strings.HasPrefix(requestPath, "/")
	switch {
	case baseSlash && requestSlash:
		return base + strings.TrimPrefix(requestPath, "/")
	case !baseSlash && !requestSlash:
		return base + "/" + requestPath
	default:
		return base + requestPath
	}
}

func joinRawQuery(base string, requestQuery string) string {
	switch {
	case base == "":
		return requestQuery
	case requestQuery == "":
		return base
	default:
		return base + "&" + requestQuery
	}
}

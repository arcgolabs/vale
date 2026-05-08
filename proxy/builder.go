package proxy

import (
	"net/http"
	"net/url"
	"strings"
	"time"

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
	fwd := oxyforward.New(false)
	fwd.Transport = NewOxyTransport()
	director := fwd.Director
	fwd.Director = func(request *http.Request) {
		RewriteRequestURL(target, request.URL)
		request.RequestURI = ""
		director(request)
	}
	return fwd
}

func Build(target *url.URL) http.Handler {
	return DefaultEngine.Build(target)
}

// NewOxyTransport returns the default upstream transport used by the built-in
// Oxy reverse proxy engine.
func NewOxyTransport() *http.Transport {
	transport := cloneDefaultTransport()
	transport.MaxIdleConns = 1024
	transport.MaxIdleConnsPerHost = 256
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = time.Second
	return transport
}

func cloneDefaultTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{Proxy: http.ProxyFromEnvironment}
	}
	return base.Clone()
}

func RewriteTargetURL(target, requestURL *url.URL) *url.URL {
	rewritten := *target
	rewritten.Path = joinURLPath(target.Path, requestURL.Path)
	rewritten.RawQuery = joinRawQuery(target.RawQuery, requestURL.RawQuery)
	return &rewritten
}

func RewriteRequestURL(target, requestURL *url.URL) {
	requestPath := requestURL.Path
	requestQuery := requestURL.RawQuery
	*requestURL = *target
	requestURL.Path = joinURLPath(target.Path, requestPath)
	requestURL.RawPath = ""
	requestURL.RawQuery = joinRawQuery(target.RawQuery, requestQuery)
}

func joinURLPath(base, requestPath string) string {
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

func joinRawQuery(base, requestQuery string) string {
	switch {
	case base == "":
		return requestQuery
	case requestQuery == "":
		return base
	default:
		return base + "&" + requestQuery
	}
}

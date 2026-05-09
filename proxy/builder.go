package proxy

import (
	"net/http"
	"net/http/httputil"
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
	base := oxyforward.New(false)
	return &httputil.ReverseProxy{
		Transport:    NewOxyTransport(),
		BufferPool:   defaultProxyBufferPool,
		ErrorHandler: base.ErrorHandler,
		Rewrite: func(rewrite *httputil.ProxyRequest) {
			originalHost := rewrite.In.Host
			RewriteRequestURL(target, rewrite.Out.URL)
			rewrite.Out.Host = target.Host
			rewrite.Out.RequestURI = ""
			rewrite.Out.Proto = "HTTP/1.1"
			rewrite.Out.ProtoMajor = 1
			rewrite.Out.ProtoMinor = 1
			rewriteForwardedHeaders(rewrite.Out, originalHost, rewrite.In.TLS != nil)
		},
	}
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
	transport.DisableCompression = true
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

func rewriteForwardedHeaders(request *http.Request, originalHost string, inboundTLS bool) {
	if request.Header.Get("X-Forwarded-Host") == "" && originalHost != "" {
		request.Header.Set("X-Forwarded-Host", originalHost)
	}
	if request.Header.Get("X-Forwarded-Proto") != "" {
		return
	}
	if inboundTLS {
		request.Header.Set("X-Forwarded-Proto", "https")
		return
	}
	request.Header.Set("X-Forwarded-Proto", "http")
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

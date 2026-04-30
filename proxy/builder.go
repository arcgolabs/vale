package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	oxyforward "github.com/vulcand/oxy/v2/forward"
)

const (
	EngineStdlib = "stdlib"
	EngineOxy    = "oxy"
)

func NormalizeEngine(engine string) string {
	engine = strings.TrimSpace(strings.ToLower(engine))
	if engine == "" {
		return EngineStdlib
	}
	return engine
}

func Build(engine string, target *url.URL) (http.Handler, error) {
	switch NormalizeEngine(engine) {
	case EngineStdlib:
		return buildStdlib(target), nil
	case EngineOxy:
		return buildOxy(target), nil
	default:
		return nil, fmt.Errorf("unsupported proxy engine %q", engine)
	}
}

func buildStdlib(target *url.URL) http.Handler {
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	return rp
}

func buildOxy(target *url.URL) http.Handler {
	fwd := oxyforward.New(true)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyReq := r.Clone(r.Context())
		proxyReq.URL = cloneURL(target)
		proxyReq.Host = target.Host
		fwd.ServeHTTP(w, proxyReq)
	})
}

func cloneURL(source *url.URL) *url.URL {
	cloned := *source
	return &cloned
}

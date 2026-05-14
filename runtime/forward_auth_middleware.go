package runtime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
)

const (
	defaultForwardAuthTimeout           = 5 * time.Second
	defaultForwardAuthBodyBytes         = 1 << 20
	defaultForwardAuthResponseBodyBytes = 1 << 20
)

var hopByHopHeaders = collectionset.NewSet(
	"connection",
	"keep-alive",
	"proxy-authenticate",
	"proxy-authorization",
	"te",
	"trailer",
	"transfer-encoding",
	"upgrade",
)

func wrapForwardAuthMiddleware(next http.Handler, middleware MiddlewareRuntime) http.Handler {
	forwardAuth := middleware.ForwardAuth
	if !forwardAuth.Enabled || forwardAuth.Address == "" {
		return next
	}
	authURL, err := url.Parse(forwardAuth.Address)
	if err != nil || authURL.Scheme == "" || authURL.Host == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		})
	}
	client := &http.Client{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, ok := authorizeForwardAuth(w, r, client, authURL.String(), forwardAuth)
		if !ok {
			if status >= http.StatusInternalServerError {
				http.Error(w, http.StatusText(status), status)
				return
			}
			return
		}
		next.ServeHTTP(w, r)
	})
}

func authorizeForwardAuth(
	w http.ResponseWriter,
	r *http.Request,
	client *http.Client,
	authAddress string,
	forwardAuth ForwardAuthRuntime,
) (int, bool) {
	body, status, err := forwardAuthBody(r, forwardAuth)
	if err != nil {
		http.Error(w, http.StatusText(status), status)
		return status, false
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, authAddress, body)
	if err != nil {
		return http.StatusInternalServerError, false
	}
	copyForwardAuthRequestHeaders(request.Header, r.Header, forwardAuth.AuthRequestHeaders)
	applyForwardedHeaders(request.Header, r, forwardAuth.TrustForwardHeader)

	response, err := doForwardAuthRequest(client, request, forwardAuth)
	if err != nil {
		return http.StatusServiceUnavailable, false
	}
	defer closeForwardAuthBody(response.Body)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		writeForwardAuthRejection(w, response, forwardAuth.MaxResponseBodyBytes)
		return response.StatusCode, false
	}
	copyForwardAuthResponseHeaders(r.Header, response.Header, forwardAuth.AuthResponseHeaders)
	return response.StatusCode, true
}

func forwardAuthBody(r *http.Request, forwardAuth ForwardAuthRuntime) (io.Reader, int, error) {
	if !forwardAuth.ForwardBody || r.Body == nil || r.Body == http.NoBody {
		return nil, http.StatusOK, nil
	}
	limit := forwardAuth.MaxBodyBytes
	if limit <= 0 {
		limit = defaultForwardAuthBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("read forward auth request body: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, http.StatusRequestEntityTooLarge, errForwardAuthBodyTooLarge{}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return bytes.NewReader(body), http.StatusOK, nil
}

func doForwardAuthRequest(
	client *http.Client,
	request *http.Request,
	forwardAuth ForwardAuthRuntime,
) (*http.Response, error) {
	timeout := forwardAuth.Timeout
	if timeout <= 0 {
		timeout = defaultForwardAuthTimeout
	}
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()
	request = request.WithContext(ctx)
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("run forward auth request: %w", err)
	}
	return response, nil
}

type errForwardAuthBodyTooLarge struct{}

func (errForwardAuthBodyTooLarge) Error() string {
	return "forward auth body too large"
}

func copyForwardAuthRequestHeaders(dst, src http.Header, allowed *collectionlist.List[string]) {
	if allowed == nil || allowed.IsEmpty() {
		copyHeader(dst, src)
		return
	}
	allowed.Range(func(_ int, header string) bool {
		copyHeaderValues(dst, src, header)
		return true
	})
}

func copyForwardAuthResponseHeaders(dst, src http.Header, allowed *collectionlist.List[string]) {
	if allowed == nil || allowed.IsEmpty() {
		return
	}
	allowed.Range(func(_ int, header string) bool {
		dst.Del(header)
		copyHeaderValues(dst, src, header)
		return true
	})
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		if hopByHopHeaders.Contains(strings.ToLower(key)) {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyHeaderValues(dst, src http.Header, key string) {
	key = strings.TrimSpace(key)
	if key == "" || hopByHopHeaders.Contains(strings.ToLower(key)) {
		return
	}
	for _, value := range src.Values(key) {
		dst.Add(key, value)
	}
}

func applyForwardedHeaders(headers http.Header, r *http.Request, trustForwardHeader bool) {
	setForwardedHeader(headers, "X-Forwarded-Method", r.Method, trustForwardHeader)
	setForwardedHeader(headers, "X-Forwarded-Proto", requestScheme(r), trustForwardHeader)
	setForwardedHeader(headers, "X-Forwarded-Host", r.Host, trustForwardHeader)
	setForwardedHeader(headers, "X-Forwarded-Uri", requestURI(r), trustForwardHeader)
	setForwardedHeader(headers, "X-Forwarded-For", remoteIP(r.RemoteAddr), trustForwardHeader)
}

func setForwardedHeader(headers http.Header, key, value string, trustForwardHeader bool) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if trustForwardHeader && strings.TrimSpace(headers.Get(key)) != "" {
		return
	}
	headers.Set(key, value)
}

func requestURI(r *http.Request) string {
	if r.URL == nil {
		return "/"
	}
	if r.URL.RequestURI() == "" {
		return "/"
	}
	return r.URL.RequestURI()
}

func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func writeForwardAuthRejection(w http.ResponseWriter, response *http.Response, maxBodyBytes int64) {
	copyHeader(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	if response.Body == nil {
		return
	}
	limit := maxBodyBytes
	if limit <= 0 {
		limit = defaultForwardAuthResponseBodyBytes
	}
	if _, err := io.Copy(w, io.LimitReader(response.Body, limit)); err != nil {
		return
	}
}

func closeForwardAuthBody(body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		return
	}
}

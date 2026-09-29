package gateway_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arcgolabs/vale/compiler"
	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/gateway"
)

func TestEventStreamOutlivesServerWriteTimeoutWithoutCompressionBuffering(t *testing.T) {
	t.Parallel()

	upstream, release := newDelayedSSEUpstream(t)
	entrypointAddr := startSSEGateway(t, upstream.URL, func(cfg *config.Config) {
		cfg.Middlewares = []config.Middleware{{
			Name:     "compress-stream",
			Compress: &config.Compress{Enabled: true, MinBytes: 1 << 20},
		}}
		cfg.Routes[0].Middlewares = []string{"compress-stream"}
		cfg.Security.WriteTimeout = "25ms"
	})
	response := openSSE(t, entrypointAddr)
	t.Cleanup(func() { closeResponseBody(t, response) })
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		t.Fatalf("content encoding = %q, want SSE compression bypass", encoding)
	}
	reader := bufio.NewReader(response.Body)
	if event := readSSEEvent(t, reader); event != "data: first\n\n" {
		t.Fatalf("first event = %q", event)
	}

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	<-timer.C
	release()
	if event := readSSEEvent(t, reader); event != "data: [DONE]\n\n" {
		t.Fatalf("final event = %q", event)
	}
}

func TestClientDisconnectCancelsEventStreamUpstream(t *testing.T) {
	t.Parallel()

	upstream, upstreamCanceled := newCancelableSSEUpstream(t)
	entrypointAddr := startSSEGateway(t, upstream.URL, nil)
	requestContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := newSSERequest(requestContext, entrypointAddr)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := response.Body.Close(); err != nil {
		t.Error(err)
	}

	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request context was not canceled after client disconnect")
	}
}

func newDelayedSSEUpstream(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	releaseDone := make(chan struct{})
	release := sync.OnceFunc(func() { close(releaseDone) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSSETestRequest(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		writeSSEEvent(t, w, "data: first\n\n")
		<-releaseDone
		writeSSEEvent(t, w, "data: [DONE]\n\n")
	}))
	t.Cleanup(server.Close)
	t.Cleanup(release)
	return server, release
}

func newCancelableSSEUpstream(t *testing.T) (*httptest.Server, <-chan struct{}) {
	t.Helper()
	upstreamCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSSETestRequest(w, r) {
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSEEvent(t, w, "data: connected\n\n")
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	t.Cleanup(server.Close)
	return server, upstreamCanceled
}

func isSSETestRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/events" {
		return true
	}
	w.WriteHeader(http.StatusNoContent)
	return false
}

func writeSSEEvent(t *testing.T, w http.ResponseWriter, event string) {
	t.Helper()
	if _, err := io.WriteString(w, event); err != nil {
		t.Error(err)
		return
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func startSSEGateway(t *testing.T, upstreamURL string, configure func(*config.Config)) string {
	t.Helper()
	entrypointAddr := freeAddr(t)
	cfg := config.Default()
	cfg.Entrypoints[0].Address = entrypointAddr
	cfg.Admin.Address = freeAddr(t)
	cfg.Services[0].Endpoints[0].URL = upstreamURL
	cfg.Health.Interval = "1h"
	if configure != nil {
		configure(cfg)
	}
	snapshot, err := compiler.Compile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	runningGateway, err := gateway.New(gateway.WithStaticSnapshot(snapshot), gateway.WithLogger(discardLogger()))
	if err != nil {
		t.Fatal(err)
	}
	if startErr := runningGateway.Start(t.Context()); startErr != nil {
		t.Fatal(startErr)
	}
	t.Cleanup(func() { stopGateway(t, runningGateway) })
	return entrypointAddr
}

func openSSE(t *testing.T, entrypointAddr string) *http.Response {
	t.Helper()
	request, err := newSSERequest(t.Context(), entrypointAddr)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func newSSERequest(ctx context.Context, entrypointAddr string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+entrypointAddr+"/events", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create SSE request: %w", err)
	}
	request.Header.Set("Accept-Encoding", "gzip")
	return request, nil
}

func closeResponseBody(t *testing.T, response *http.Response) {
	t.Helper()
	if err := response.Body.Close(); err != nil {
		t.Error(err)
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var event strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event: %v", err)
		}
		if _, writeErr := event.WriteString(line); writeErr != nil {
			t.Fatal(writeErr)
		}
		if line == "\n" {
			return event.String()
		}
	}
}

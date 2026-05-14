package runtime_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	valeruntime "github.com/arcgolabs/vale/runtime"
)

func TestForwardAuthAllowsValidRequestAndCopiesHeaders(t *testing.T) {
	t.Parallel()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ok" {
			t.Errorf("authorization = %q", got)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.Header.Get("X-Forwarded-Uri"); got != "/api?tenant=acme" {
			t.Errorf("x-forwarded-uri = %q", got)
		}
		w.Header().Set("X-Authenticated-Subject", "demo-user")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(auth.Close)

	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Authenticated-Subject"); got != "demo-user" {
			t.Fatalf("authenticated subject = %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}), collectionlist.NewList(valeruntime.MiddlewareRuntime{
		Name: "auth",
		ForwardAuth: valeruntime.ForwardAuthRuntime{
			Enabled:             true,
			Address:             auth.URL,
			AuthRequestHeaders:  collectionlist.NewList("Authorization"),
			AuthResponseHeaders: collectionlist.NewList("X-Authenticated-Subject"),
		},
	}))

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"http://gateway.local/api?tenant=acme",
		http.NoBody,
	)
	request.Header.Set("Authorization", "Bearer ok")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}
}

func TestForwardAuthRejectsBeforeBackend(t *testing.T) {
	t.Parallel()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "denied", http.StatusUnauthorized)
	}))
	t.Cleanup(auth.Close)

	var backendCalled atomic.Bool
	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		backendCalled.Store(true)
	}), collectionlist.NewList(valeruntime.MiddlewareRuntime{
		Name: "auth",
		ForwardAuth: valeruntime.ForwardAuthRuntime{
			Enabled: true,
			Address: auth.URL,
		},
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(
		response,
		httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://gateway.local/api", http.NoBody),
	)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if backendCalled.Load() {
		t.Fatal("backend was called")
	}
	if !strings.Contains(response.Body.String(), "denied") {
		t.Fatalf("body = %q", response.Body.String())
	}
}

func TestForwardAuthCanForwardAndReplayRequestBody(t *testing.T) {
	t.Parallel()

	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read auth body: %v", err)
		}
		if string(body) != "payload" {
			t.Errorf("auth body = %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(auth.Close)

	handler := valeruntime.WrapMiddlewares(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read backend body: %v", err)
		}
		if string(body) != "payload" {
			t.Fatalf("backend body = %q", body)
		}
		w.WriteHeader(http.StatusOK)
	}), collectionlist.NewList(valeruntime.MiddlewareRuntime{
		Name: "auth",
		ForwardAuth: valeruntime.ForwardAuthRuntime{
			Enabled:      true,
			Address:      auth.URL,
			ForwardBody:  true,
			MaxBodyBytes: 32,
		},
	}))

	response := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"http://gateway.local/api",
		strings.NewReader("payload"),
	)
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

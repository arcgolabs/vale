package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arcgolabs/vale"
)

func TestRemoteJWTMiddleware(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	auth, err := newDemoAuthService(logger)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	token, err := auth.IssueToken("demo-user")
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	authServer := httptest.NewServer(auth.Handler())
	t.Cleanup(authServer.Close)

	handler := remoteJWTMiddleware{
		ValidateURL: authServer.URL + "/validate",
		Client:      authServer.Client(),
	}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Authenticated-Subject"); got != "demo-user" {
			t.Fatalf("subject = %q", got)
		}
		w.WriteHeader(http.StatusOK)
	}), vale.RuntimeMiddleware{})

	assertJWTMiddlewareStatus(t, handler, "", http.StatusUnauthorized)
	assertJWTMiddlewareStatus(t, handler, "Bearer "+token, http.StatusOK)

	authServer.Close()
	assertJWTMiddlewareStatus(t, handler, "Bearer "+token, http.StatusServiceUnavailable)
}

func assertJWTMiddlewareStatus(t *testing.T, handler http.Handler, authorization string, expected int) {
	t.Helper()
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://gateway.local/api", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != expected {
		t.Fatalf("status = %d, want %d", response.Code, expected)
	}
}

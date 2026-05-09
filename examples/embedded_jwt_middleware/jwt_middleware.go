package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/arcgolabs/vale"
)

type remoteJWTMiddleware struct {
	ValidateURL string
	Client      *http.Client
}

func (m remoteJWTMiddleware) Middleware(next http.Handler, _ vale.RuntimeMiddleware) http.Handler {
	client := m.httpClient()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		subject, status, err := m.validate(r.Context(), client, token)
		if err != nil {
			http.Error(w, http.StatusText(status), status)
			return
		}
		if subject != "" {
			r.Header.Set("X-Authenticated-Subject", subject)
		}
		next.ServeHTTP(w, r)
	})
}

func (m remoteJWTMiddleware) httpClient() *http.Client {
	if m.Client != nil {
		return m.Client
	}
	return &http.Client{Timeout: authClientTimeout}
}

func (m remoteJWTMiddleware) validate(ctx context.Context, client *http.Client, token string) (string, int, error) {
	body, err := json.Marshal(tokenValidationRequest{Token: token})
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("encode auth request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.ValidateURL, bytes.NewReader(body))
	if err != nil {
		return "", http.StatusInternalServerError, fmt.Errorf("build auth request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return "", http.StatusServiceUnavailable, fmt.Errorf("call auth service: %w", err)
	}
	defer closeBody(response.Body)
	if response.StatusCode != http.StatusOK {
		return "", http.StatusUnauthorized, errInvalidToken
	}

	var validation tokenValidationResponse
	if err := json.NewDecoder(response.Body).Decode(&validation); err != nil {
		return "", http.StatusServiceUnavailable, fmt.Errorf("decode auth response: %w", err)
	}
	if !validation.Active {
		return "", http.StatusUnauthorized, errInvalidToken
	}
	return validation.Subject, http.StatusOK, nil
}

func bearerToken(authorization string) (string, bool) {
	token, ok := strings.CutPrefix(strings.TrimSpace(authorization), "Bearer ")
	token = strings.TrimSpace(token)
	return token, ok && token != ""
}

func closeBody(body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		return
	}
}

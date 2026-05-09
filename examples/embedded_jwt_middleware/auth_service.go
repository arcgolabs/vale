package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const maxAuthRequestBytes = 1 << 20

var errInvalidToken = errors.New("invalid token")

type (
	authServiceConfig struct {
		SigningKey []byte
		Issuer     string
		Audience   string
		Leeway     time.Duration
		Store      *tokenStore
		Logger     *slog.Logger
	}
	authService struct {
		signingKey []byte
		issuer     string
		audience   string
		leeway     time.Duration
		tokens     *tokenStore
		logger     *slog.Logger
	}
	tokenStore struct {
		tokens *mapping.ConcurrentMap[string, tokenRecord]
	}
	tokenRecord struct {
		Subject   string    `json:"subject"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	tokenValidationRequest struct {
		Token string `json:"token"`
	}
	tokenValidationResponse struct {
		Active    bool      `json:"active"`
		Subject   string    `json:"subject,omitempty"`
		ExpiresAt time.Time `json:"expires_at,omitzero"`
	}
)

func newAuthService(config authServiceConfig) *authService {
	store := config.Store
	if store == nil {
		store = newTokenStore()
	}
	logger := config.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &authService{
		signingKey: config.SigningKey,
		issuer:     config.Issuer,
		audience:   config.Audience,
		leeway:     config.Leeway,
		tokens:     store,
		logger:     logger,
	}
}

func newTokenStore() *tokenStore {
	return &tokenStore{tokens: mapping.NewConcurrentMap[string, tokenRecord]()}
}

func (s *tokenStore) Put(token string, record tokenRecord) {
	if s == nil || s.tokens == nil {
		return
	}
	s.tokens.Set(token, record)
}

func (s *tokenStore) Get(token string) (tokenRecord, bool) {
	if s == nil || s.tokens == nil {
		return tokenRecord{}, false
	}
	record, ok := s.tokens.Get(token)
	if !ok {
		return tokenRecord{}, false
	}
	if !record.ExpiresAt.IsZero() && time.Now().After(record.ExpiresAt) {
		s.tokens.Delete(token)
		return tokenRecord{}, false
	}
	return record, true
}

func (s *authService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/validate", s.handleValidate)
	return mux
}

func (s *authService) IssueToken(subject string) (string, error) {
	now := time.Now()
	claims := jwt.Claims{
		Issuer:    s.issuer,
		Subject:   subject,
		Audience:  jwt.Audience{s.audience},
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
		Expiry:    jwt.NewNumericDate(now.Add(15 * time.Minute)),
	}
	rawToken, err := signClaims(s.signingKey, claims)
	if err != nil {
		return "", err
	}
	s.tokens.Put(rawToken, tokenRecord{
		Subject:   subject,
		ExpiresAt: claims.Expiry.Time(),
	})
	return rawToken, nil
}

func (s *authService) ValidateToken(rawToken string) (tokenRecord, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return tokenRecord{}, errInvalidToken
	}
	token, err := jwt.ParseSigned(rawToken, []jose.SignatureAlgorithm{jose.HS256})
	if err != nil {
		return tokenRecord{}, fmt.Errorf("parse jwt: %w", err)
	}
	var claims jwt.Claims
	if err := token.Claims(s.signingKey, &claims); err != nil {
		return tokenRecord{}, fmt.Errorf("verify jwt signature: %w", err)
	}
	expected := jwt.Expected{
		Issuer:      s.issuer,
		AnyAudience: jwt.Audience{s.audience},
		Time:        time.Now(),
	}
	if err := claims.ValidateWithLeeway(expected, s.leeway); err != nil {
		return tokenRecord{}, fmt.Errorf("validate jwt claims: %w", err)
	}
	record, ok := s.tokens.Get(rawToken)
	if !ok || record.Subject != claims.Subject {
		return tokenRecord{}, errInvalidToken
	}
	return record, nil
}

func (s *authService) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	var request tokenValidationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	record, err := s.ValidateToken(request.Token)
	if err != nil {
		s.writeValidation(w, http.StatusUnauthorized, tokenValidationResponse{Active: false})
		return
	}
	s.writeValidation(w, http.StatusOK, tokenValidationResponse{
		Active:    true,
		Subject:   record.Subject,
		ExpiresAt: record.ExpiresAt,
	})
}

func (s *authService) writeValidation(w http.ResponseWriter, status int, response tokenValidationResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Warn("write auth validation response failed", "error", err)
	}
}

func newSigningKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate jwt signing key: %w", err)
	}
	return key, nil
}

func signClaims(key []byte, claims jwt.Claims) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", fmt.Errorf("create jwt signer: %w", err)
	}
	rawToken, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		return "", fmt.Errorf("sign jwt: %w", err)
	}
	return rawToken, nil
}

func startAuthService(ctx context.Context, auth *authService, logger *slog.Logger) (string, func(), error) {
	return startHTTPServer(ctx, logger, "auth service", auth.Handler())
}

// JWT auth middleware extension: register a custom gateway middleware that
// calls an external auth service before forwarding requests to the backend.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arcgolabs/vale"
)

const (
	jwtMiddlewareType = "jwt_auth"
	jwtIssuer         = "https://auth.example.local"
	jwtAudience       = "vale-api"
	jwtLeeway         = 5 * time.Second
	authClientTimeout = 750 * time.Millisecond
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(context.Background(), logger); err != nil {
		logger.Error("embedded gateway failed", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backendAddr, stopBackend, err := startBackend(parent, logger)
	if err != nil {
		return err
	}
	defer stopBackend()

	auth, err := newDemoAuthService(logger)
	if err != nil {
		return err
	}
	demoToken, err := auth.IssueToken("demo-user")
	if err != nil {
		return err
	}
	authAddr, stopAuth, err := startAuthService(parent, auth, logger)
	if err != nil {
		return err
	}
	defer stopAuth()

	validator := remoteJWTMiddleware{
		ValidateURL: "http://" + authAddr + "/validate",
		Client:      &http.Client{Timeout: authClientTimeout},
	}
	registry := vale.NewRegistry()
	if registerErr := registry.RegisterMiddleware(jwtMiddlewareType, validator.Middleware); registerErr != nil {
		return fmt.Errorf("register jwt middleware: %w", registerErr)
	}

	cfg := vale.NewConfigBuilder().
		Entrypoint("web", ":8080").
		MiddlewareNamed("jwt", vale.MiddlewareType(jwtMiddlewareType)).
		Service("api", "http://"+backendAddr).
		RouteTo("api", "web", "api", vale.RoutePathPrefix("/api"), vale.RouteMiddlewares("jwt")).
		Admin(":19090").
		Observability(true, true).
		Build()

	embeddedGateway, err := vale.New(
		vale.WithLogger(logger),
		vale.WithRegistry(registry),
		vale.WithStaticConfig(cfg),
	)
	if err != nil {
		return fmt.Errorf("create embedded gateway: %w", err)
	}
	if startErr := embeddedGateway.Start(ctx); startErr != nil {
		return fmt.Errorf("start embedded gateway: %w", startErr)
	}

	logger.Info(
		"embedded gateway started",
		"gateway", "http://127.0.0.1:8080/api",
		"auth", "http://"+authAddr+"/validate",
		"admin", "http://127.0.0.1:19090",
		"valid_curl", fmt.Sprintf("curl -H %q http://127.0.0.1:8080/api", "Authorization: Bearer "+demoToken),
		"invalid_curl", "curl http://127.0.0.1:8080/api",
	)

	<-ctx.Done()
	if err := stopGateway(parent, logger, embeddedGateway); err != nil {
		return err
	}
	return nil
}

func newDemoAuthService(logger *slog.Logger) (*authService, error) {
	signingKey, err := newSigningKey()
	if err != nil {
		return nil, err
	}
	return newAuthService(authServiceConfig{
		SigningKey: signingKey,
		Issuer:     jwtIssuer,
		Audience:   jwtAudience,
		Leeway:     jwtLeeway,
		Store:      newTokenStore(),
		Logger:     logger,
	}), nil
}

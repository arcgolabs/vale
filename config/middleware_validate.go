package config

import (
	"errors"
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
)

type middlewareValidator func(*Middleware) error

var middlewareValidators = collectionlist.NewList[middlewareValidator](
	validateRateLimitPolicy,
	validateCORSPolicy,
	validateSecurePolicy,
)

func validateMiddleware(middleware *Middleware, middlewareSet *collectionset.Set[string]) error {
	if err := validateMiddlewareIdentity(middleware, middlewareSet); err != nil {
		return err
	}
	middlewareErr := error(nil)
	middlewareValidators.Range(func(_ int, validate middlewareValidator) bool {
		middlewareErr = validate(middleware)
		return middlewareErr == nil
	})
	return middlewareErr
}

func validateMiddlewareIdentity(middleware *Middleware, middlewareSet *collectionset.Set[string]) error {
	if middleware.Name == "" {
		return errors.New("middleware name cannot be empty")
	}
	if middlewareSet.Contains(middleware.Name) {
		return fmt.Errorf("duplicated middleware %q", middleware.Name)
	}
	return nil
}

func validateRateLimitPolicy(middleware *Middleware) error {
	if middleware.RateLimit == nil {
		return nil
	}
	if middleware.RateLimit.Rate < 0 {
		return fmt.Errorf("middleware %q rate_limit rate cannot be negative", middleware.Name)
	}
	if middleware.RateLimit.Burst < 0 {
		return fmt.Errorf("middleware %q rate_limit burst cannot be negative", middleware.Name)
	}
	return nil
}

func validateCORSPolicy(middleware *Middleware) error {
	if middleware.CORS == nil || middleware.CORS.OptionsSuccessStatus >= 0 {
		return nil
	}
	return fmt.Errorf("middleware %q cors options_success_status cannot be negative", middleware.Name)
}

func validateSecurePolicy(middleware *Middleware) error {
	if middleware.Secure == nil || middleware.Secure.STSSeconds >= 0 {
		return nil
	}
	return fmt.Errorf("middleware %q secure sts_seconds cannot be negative", middleware.Name)
}

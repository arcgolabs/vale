package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
)

type middlewareValidator func(*Middleware) error

var middlewareValidators = collectionlist.NewList[middlewareValidator](
	validateRateLimitPolicy,
	validateCORSPolicy,
	validateSecurePolicy,
	validateCompressPolicy,
	validateIPAllowListPolicy,
	validateForwardAuthPolicy,
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

func validateCompressPolicy(middleware *Middleware) error {
	if middleware.Compress == nil || middleware.Compress.MinBytes >= 0 {
		return nil
	}
	return fmt.Errorf("middleware %q compress min_bytes cannot be negative", middleware.Name)
}

func validateIPAllowListPolicy(middleware *Middleware) error {
	if middleware.IPAllowList == nil {
		return nil
	}
	for _, source := range middleware.IPAllowList.SourceRange {
		source = strings.TrimSpace(source)
		if source == "" {
			continue
		}
		if ip := net.ParseIP(source); ip != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(source); err != nil {
			return fmt.Errorf("middleware %q ip_allow_list source_range %q is not an IP or CIDR", middleware.Name, source)
		}
	}
	return nil
}

func validateForwardAuthPolicy(middleware *Middleware) error {
	if middleware.ForwardAuth == nil {
		return nil
	}
	if err := validateForwardAuthAddress(middleware.Name, middleware.ForwardAuth.Address); err != nil {
		return err
	}
	if err := validateForwardAuthTimeout(middleware.Name, middleware.ForwardAuth.Timeout); err != nil {
		return err
	}
	return validateForwardAuthLimits(middleware.Name, middleware.ForwardAuth)
}

func validateForwardAuthAddress(middlewareName, rawAddress string) error {
	address := strings.TrimSpace(rawAddress)
	if address == "" {
		return fmt.Errorf("middleware %q forward_auth address cannot be empty", middlewareName)
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("middleware %q forward_auth address %q is invalid", middlewareName, address)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("middleware %q forward_auth address scheme must be http or https", middlewareName)
	}
	return nil
}

func validateForwardAuthTimeout(middlewareName, rawTimeout string) error {
	timeout := strings.TrimSpace(rawTimeout)
	if timeout == "" {
		return nil
	}
	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil || parsedTimeout <= 0 {
		return fmt.Errorf("middleware %q forward_auth timeout %q is invalid", middlewareName, timeout)
	}
	return nil
}

func validateForwardAuthLimits(middlewareName string, forwardAuth *ForwardAuth) error {
	if forwardAuth.MaxBodyBytes < 0 {
		return fmt.Errorf("middleware %q forward_auth max_body_bytes cannot be negative", middlewareName)
	}
	if forwardAuth.MaxResponseBodyBytes < 0 {
		return fmt.Errorf("middleware %q forward_auth max_response_body_bytes cannot be negative", middlewareName)
	}
	return nil
}

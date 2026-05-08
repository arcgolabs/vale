package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type config struct {
	targets      *collectionlist.List[target]
	duration     time.Duration
	warmup       time.Duration
	concurrency  int
	timeout      time.Duration
	method       string
	path         string
	jsonPath     string
	markdownPath string
	logLevel     string
}

type target struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func parseConfig() (config, error) {
	targets := flag.String(
		"target",
		"vale=http://127.0.0.1:18080,traefik=http://127.0.0.1:18081,caddy=http://127.0.0.1:18082",
		"comma-separated targets in name=url form",
	)
	duration := flag.Duration("duration", 15*time.Second, "measured duration per target")
	warmup := flag.Duration("warmup", 3*time.Second, "warmup duration per target")
	concurrency := flag.Int("concurrency", 32, "concurrent clients")
	timeout := flag.Duration("timeout", 5*time.Second, "request timeout")
	method := flag.String("method", http.MethodGet, "HTTP method")
	path := flag.String("path", "/", "request path")
	jsonPath := flag.String("json", "", "optional JSON report path")
	markdownPath := flag.String("markdown", "", "optional Markdown report path")
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error, off")
	flag.Parse()

	targetList, err := parseTargets(*targets, *path)
	if err != nil {
		return config{}, err
	}
	if err := validatePositiveDuration("duration", *duration); err != nil {
		return config{}, err
	}
	if err := validatePositiveDuration("timeout", *timeout); err != nil {
		return config{}, err
	}
	if *concurrency <= 0 {
		return config{}, errors.New("concurrency must be positive")
	}
	return config{
		targets:      targetList,
		duration:     *duration,
		warmup:       maxDuration(*warmup, 0),
		concurrency:  *concurrency,
		timeout:      *timeout,
		method:       strings.ToUpper(strings.TrimSpace(*method)),
		path:         normalizePath(*path),
		jsonPath:     strings.TrimSpace(*jsonPath),
		markdownPath: strings.TrimSpace(*markdownPath),
		logLevel:     strings.ToLower(strings.TrimSpace(*logLevel)),
	}, nil
}

func validatePositiveDuration(name string, value time.Duration) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func parseTargets(rawTargets, requestPath string) (*collectionlist.List[target], error) {
	parts := collectionlist.NewList(strings.Split(rawTargets, ",")...)
	targets := collectionlist.NewListWithCapacity[target](parts.Len())
	seen := mapping.NewMap[string, struct{}]()
	var parseErr error
	parts.Range(func(_ int, rawTarget string) bool {
		parsedTarget, err := parseTarget(rawTarget, requestPath)
		if err != nil {
			parseErr = err
			return false
		}
		if _, exists := seen.Get(parsedTarget.Name); exists {
			parseErr = fmt.Errorf("target %q is duplicated", parsedTarget.Name)
			return false
		}
		seen.Set(parsedTarget.Name, struct{}{})
		targets.Add(parsedTarget)
		return true
	})
	if parseErr != nil {
		return nil, parseErr
	}
	if targets.IsEmpty() {
		return nil, errors.New("at least one target is required")
	}
	return targets, nil
}

func parseTarget(rawTarget, requestPath string) (target, error) {
	pair := strings.SplitN(rawTarget, "=", 2)
	if len(pair) != 2 {
		return target{}, fmt.Errorf("target %q must use name=url form", rawTarget)
	}
	name := strings.TrimSpace(pair[0])
	rawURL := joinURLPath(strings.TrimSpace(pair[1]), requestPath)
	if name == "" || rawURL == "" {
		return target{}, fmt.Errorf("target %q has empty name or url", rawTarget)
	}
	if parsed, err := url.Parse(rawURL); err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return target{}, fmt.Errorf("target %q url %q is invalid", name, rawURL)
	}
	return target{Name: name, URL: rawURL}, nil
}

func joinURLPath(rawURL, requestPath string) string {
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	return rawURL + normalizePath(requestPath)
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if strings.HasPrefix(path, "/") {
		return path
	}
	return "/" + path
}

func maxDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}

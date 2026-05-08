// Command proxybench runs a lightweight HTTP reverse-proxy benchmark.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const toolName = "vale-proxybench"

func main() {
	cfg, err := parseConfig()
	if err != nil {
		exitWithError(2, err)
	}
	logger, err := newLogger(cfg.logLevel)
	if err != nil {
		exitWithError(2, err)
	}
	if err := run(cfg, logger); err != nil {
		exitWithError(1, err)
	}
}

func newLogger(level string) (*slog.Logger, error) {
	var slogLevel slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		slogLevel = slog.LevelInfo
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	case "off":
		return slog.New(slog.DiscardHandler), nil
	default:
		return nil, fmt.Errorf("unsupported log level %q", level)
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slogLevel})), nil
}

func exitWithError(code int, err error) {
	if _, writeErr := fmt.Fprintf(os.Stderr, "proxybench: %v\n", err); writeErr != nil {
		os.Exit(1)
	}
	os.Exit(code)
}

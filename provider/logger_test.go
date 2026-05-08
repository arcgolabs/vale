package provider_test

import (
	"log/slog"
	"testing"

	"github.com/arcgolabs/vale/provider"
)

type loggerAwareStub struct {
	logger *slog.Logger
}

func (s *loggerAwareStub) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

func TestApplyLogger(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)
	target := &loggerAwareStub{}

	provider.ApplyLogger(target, logger)

	if target.logger != logger {
		t.Fatalf("expected logger to be applied")
	}
}

func TestApplyLoggerIgnoresUnsupportedTarget(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	provider.ApplyLogger(struct{}{}, logger)
	provider.ApplyLogger(nil, logger)
	provider.ApplyLogger(&loggerAwareStub{}, nil)
}

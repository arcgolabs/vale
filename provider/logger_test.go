package provider

import (
	"io"
	"log/slog"
	"testing"
)

type loggerAwareStub struct {
	logger *slog.Logger
}

func (s *loggerAwareStub) SetLogger(logger *slog.Logger) {
	s.logger = logger
}

func TestApplyLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	target := &loggerAwareStub{}

	ApplyLogger(target, logger)

	if target.logger != logger {
		t.Fatalf("expected logger to be applied")
	}
}

func TestApplyLoggerIgnoresUnsupportedTarget(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ApplyLogger(struct{}{}, logger)
	ApplyLogger(nil, logger)
	ApplyLogger(&loggerAwareStub{}, nil)
}

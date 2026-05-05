package vela

import (
	"io"
	"log/slog"
	"testing"

	"github.com/arcgolabs/vela/config"
)

func TestNewUsesDefaultConfigWhenNoSourceIsConfigured(t *testing.T) {
	t.Parallel()

	gateway, err := New(WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	if gateway == nil {
		t.Fatal("New returned nil gateway")
	}
}

func TestDefaultConfigModelIsValid(t *testing.T) {
	t.Parallel()

	if err := config.Validate(config.Default()); err != nil {
		t.Fatal(err)
	}
}

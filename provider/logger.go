package provider

import "log/slog"

// LoggerAware marks components that can receive the gateway-level logger.
type LoggerAware interface {
	SetLogger(*slog.Logger)
}

// ApplyLogger injects logger when target supports [LoggerAware].
func ApplyLogger(target any, logger *slog.Logger) {
	if target == nil || logger == nil {
		return
	}
	if aware, ok := target.(LoggerAware); ok {
		aware.SetLogger(logger)
	}
}

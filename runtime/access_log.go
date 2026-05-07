package runtime

import "log/slog"

type AccessEvent struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	Host       string `json:"host"`
	StatusCode int    `json:"status_code"`
	DurationMs int64  `json:"duration_ms"`
	Route      string `json:"route"`
	Service    string `json:"service"`
	Endpoint   string `json:"endpoint"`
	UserAgent  string `json:"user_agent"`
	RemoteAddr string `json:"remote_addr"`
}

type AccessLogger struct {
	enabled bool
	logger  *slog.Logger
}

func NewAccessLogger(logger *slog.Logger, enabled bool) *AccessLogger {
	return &AccessLogger{
		enabled: enabled,
		logger:  logger,
	}
}

func (l *AccessLogger) Log(event AccessEvent) {
	if !l.enabled || l.logger == nil {
		return
	}
	l.logger.Info("access",
		slog.String("method", event.Method),
		slog.String("path", event.Path),
		slog.String("host", event.Host),
		slog.Int("status_code", event.StatusCode),
		slog.Int64("duration_ms", event.DurationMs),
		slog.String("route", event.Route),
		slog.String("service", event.Service),
		slog.String("endpoint", event.Endpoint),
		slog.String("user_agent", event.UserAgent),
		slog.String("remote_addr", event.RemoteAddr),
	)
}

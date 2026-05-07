package gateway

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/samber/mo"
)

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		slog.Default().Error("json response encode failed", "error", err)
	}
}

func parseDurationDefault(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	return mo.TupleToOption(duration, err == nil).OrElse(fallback)
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

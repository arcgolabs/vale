package gateway

import (
	"time"

	"github.com/samber/mo"
)

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

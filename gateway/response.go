package gateway

import (
	"strings"
	"time"

	"github.com/samber/mo"
)

func parseDurationDefault(value string, fallback time.Duration) time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	return mo.TupleToOption(duration, err == nil).OrElse(fallback)
}

func maxInt(value, fallback int) int {
	return mo.TupleToOption(value, value > 0).OrElse(fallback)
}

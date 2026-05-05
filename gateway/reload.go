package gateway

import (
	"maps"
	"slices"

	"github.com/arcgolabs/gateway/runtime"
)

func staticRuntimeChanges(current *runtime.CompiledSnapshot, next *runtime.CompiledSnapshot) []string {
	if current == nil || next == nil {
		return nil
	}

	changes := make([]string, 0, 6)
	if !maps.Equal(current.Entrypoints, next.Entrypoints) {
		changes = append(changes, "entrypoints")
	}
	if current.AdminAddress != next.AdminAddress {
		changes = append(changes, "admin_address")
	}
	if current.AccessLogEnabled != next.AccessLogEnabled {
		changes = append(changes, "access_log_enabled")
	}
	if current.MetricsEnabled != next.MetricsEnabled {
		changes = append(changes, "metrics_enabled")
	}
	if current.HealthInterval != next.HealthInterval {
		changes = append(changes, "health_interval")
	}
	if current.HealthTimeout != next.HealthTimeout {
		changes = append(changes, "health_timeout")
	}
	slices.Sort(changes)
	return changes
}

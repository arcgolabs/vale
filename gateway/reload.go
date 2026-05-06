package gateway

import (
	"reflect"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vela/runtime"
)

func staticRuntimeChanges(current *runtime.CompiledSnapshot, next *runtime.CompiledSnapshot) *collectionlist.List[string] {
	if current == nil || next == nil {
		return nil
	}

	changes := collectionlist.NewListWithCapacity[string](8)
	if !reflect.DeepEqual(current.Entrypoints.All(), next.Entrypoints.All()) {
		changes.Add("entrypoints")
	}
	if !reflect.DeepEqual(current.EntrypointConfigs.All(), next.EntrypointConfigs.All()) {
		changes.Add("entrypoint_configs")
	}
	if current.AdminAddress != next.AdminAddress {
		changes.Add("admin_address")
	}
	if current.AccessLogEnabled != next.AccessLogEnabled {
		changes.Add("access_log_enabled")
	}
	if current.MetricsEnabled != next.MetricsEnabled {
		changes.Add("metrics_enabled")
	}
	if current.HealthInterval != next.HealthInterval {
		changes.Add("health_interval")
	}
	if current.HealthTimeout != next.HealthTimeout {
		changes.Add("health_timeout")
	}
	if current.Security != next.Security {
		changes.Add("security")
	}
	changes.Sort(strings.Compare)
	return changes
}

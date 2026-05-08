package gateway

import (
	"reflect"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/runtime"
)

func staticRuntimeChanges(current, next *runtime.CompiledSnapshot) *collectionlist.List[string] {
	if current == nil || next == nil {
		return nil
	}

	changes := collectionlist.NewListWithCapacity[string](8)
	addRuntimeChange(changes, !reflect.DeepEqual(current.Entrypoints.All(), next.Entrypoints.All()), "entrypoints")
	addRuntimeChange(changes, !reflect.DeepEqual(current.EntrypointConfigs.All(), next.EntrypointConfigs.All()), "entrypoint_configs")
	addRuntimeChange(changes, current.AdminAddress != next.AdminAddress, "admin_address")
	addRuntimeChange(changes, current.AccessLogEnabled != next.AccessLogEnabled, "access_log_enabled")
	addRuntimeChange(changes, current.MetricsEnabled != next.MetricsEnabled, "metrics_enabled")
	addRuntimeChange(changes, current.HealthInterval != next.HealthInterval, "health_interval")
	addRuntimeChange(changes, current.HealthTimeout != next.HealthTimeout, "health_timeout")
	addRuntimeChange(changes, current.Security != next.Security, "security")
	changes.Sort(strings.Compare)
	return changes
}

func addRuntimeChange(changes *collectionlist.List[string], changed bool, name string) {
	if changed {
		changes.Add(name)
	}
}

package gateway

import (
	"maps"
	"slices"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/vale/runtime"
)

func staticRuntimeChanges(current, next *runtime.CompiledSnapshot) *collectionlist.List[string] {
	if current == nil || next == nil {
		return nil
	}

	changes := collectionlist.NewListWithCapacity[string](8)
	addRuntimeChange(changes, !maps.Equal(current.Entrypoints.All(), next.Entrypoints.All()), "entrypoints")
	addRuntimeChange(changes, !maps.EqualFunc(
		current.EntrypointConfigs.All(),
		next.EntrypointConfigs.All(),
		entrypointRuntimeEqual,
	), "entrypoint_configs")
	addRuntimeChange(changes, current.AdminAddress != next.AdminAddress, "admin_address")
	addRuntimeChange(changes, current.AccessLogEnabled != next.AccessLogEnabled, "access_log_enabled")
	addRuntimeChange(changes, current.MetricsEnabled != next.MetricsEnabled, "metrics_enabled")
	addRuntimeChange(changes, current.HealthInterval != next.HealthInterval, "health_interval")
	addRuntimeChange(changes, current.HealthTimeout != next.HealthTimeout, "health_timeout")
	addRuntimeChange(changes, current.Security != next.Security, "security")
	changes.Sort(strings.Compare)
	return changes
}

func entrypointRuntimeEqual(left, right runtime.EntrypointRuntime) bool {
	return left.Name == right.Name &&
		left.Address == right.Address &&
		left.Protocol == right.Protocol &&
		left.TLS.Enabled == right.TLS.Enabled &&
		left.TLS.CertFile == right.TLS.CertFile &&
		left.TLS.KeyFile == right.TLS.KeyFile &&
		left.TLS.ACME.Enabled == right.TLS.ACME.Enabled &&
		left.TLS.ACME.Email == right.TLS.ACME.Email &&
		left.TLS.ACME.CacheDir == right.TLS.ACME.CacheDir &&
		stringListEqual(left.TLS.ACME.Domains, right.TLS.ACME.Domains)
}

func stringListEqual(left, right *collectionlist.List[string]) bool {
	if left == nil || right == nil {
		return (left == nil || left.IsEmpty()) && (right == nil || right.IsEmpty())
	}
	return slices.Equal(left.Values(), right.Values())
}

func addRuntimeChange(changes *collectionlist.List[string], changed bool, name string) {
	if changed {
		changes.Add(name)
	}
}

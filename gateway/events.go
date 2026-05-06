package gateway

import collectionlist "github.com/arcgolabs/collectionx/list"

const EventNameStaticRuntimeConfigChanged = "gateway.static_runtime_config.changed"

// StaticRuntimeConfigChangedEvent is emitted when a hot-reloaded snapshot changes
// settings that are bound to process lifecycle resources and require restart.
type StaticRuntimeConfigChangedEvent struct {
	Fields *collectionlist.List[string]
}

func (e StaticRuntimeConfigChangedEvent) Name() string {
	return EventNameStaticRuntimeConfigChanged
}

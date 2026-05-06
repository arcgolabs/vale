package gateway

import (
	"context"

	"github.com/arcgolabs/vela/provider"
)

const EventNameStaticRuntimeConfigChanged = "gateway.static_runtime_config.changed"

// StaticRuntimeConfigChangedEvent is emitted when a hot-reloaded snapshot changes
// settings that are bound to process lifecycle resources and require restart.
type StaticRuntimeConfigChangedEvent struct {
	Fields []string
}

func (e StaticRuntimeConfigChangedEvent) Name() string {
	return EventNameStaticRuntimeConfigChanged
}

type noopEventBus struct{}

func (noopEventBus) Publish(context.Context, provider.Event) error {
	return nil
}

func (noopEventBus) Close() error {
	return nil
}

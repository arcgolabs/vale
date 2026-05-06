package eventx

import (
	"context"

	arcgoeventx "github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vela/gateway"
	"github.com/arcgolabs/vela/provider"
)

type Bus struct {
	runtime arcgoeventx.BusRuntime
}

func New(runtime arcgoeventx.BusRuntime) Bus {
	if runtime == nil {
		runtime = arcgoeventx.New()
	}
	return Bus{runtime: runtime}
}

func WithEventBus(runtime arcgoeventx.BusRuntime) gateway.Option {
	return gateway.WithEventBus(New(runtime))
}

func (b Bus) Publish(ctx context.Context, event provider.Event) error {
	return b.runtime.Publish(ctx, event)
}

func (b Bus) Close() error {
	return b.runtime.Close()
}

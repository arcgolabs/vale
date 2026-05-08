package provider

import (
	"context"
	"io"

	"github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/runtime"
)

type SnapshotProvider interface {
	Load(context.Context) (*runtime.CompiledSnapshot, error)
	Watch(context.Context, func(*runtime.CompiledSnapshot), func(error)) (io.Closer, error)
}

type ConfigProvider interface {
	Load(context.Context) (*config.Config, error)
	Watch(context.Context, func(), func(error)) (io.Closer, error)
}

type Named interface {
	Name() string
}

func ConfigProviderName(provider ConfigProvider, fallback string) string {
	if provider == nil {
		return fallback
	}
	named, ok := any(provider).(Named)
	if !ok {
		return fallback
	}
	name := named.Name()
	if name == "" {
		return fallback
	}
	return name
}

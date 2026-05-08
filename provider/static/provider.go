package static

import (
	"context"
	"io"

	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/runtime"
)

type Provider struct {
	snapshot *runtime.CompiledSnapshot
}

func New(snapshot *runtime.CompiledSnapshot) *Provider {
	return &Provider{snapshot: snapshot}
}

func (p *Provider) Load(context.Context) (*runtime.CompiledSnapshot, error) {
	return p.snapshot, nil
}

func (p *Provider) Watch(context.Context, func(*runtime.CompiledSnapshot), func(error)) (io.Closer, error) {
	return provider.NopCloser{}, nil
}

package static

import (
	"context"
	"io"

	"github.com/arcgolabs/gateway/runtime"
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
	return nopCloser{}, nil
}

type nopCloser struct{}

func (nopCloser) Close() error {
	return nil
}

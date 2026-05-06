package provider

import (
	"io"
	"sync"
)

// NopCloser is an io.Closer that does nothing.
type NopCloser struct{}

func (NopCloser) Close() error {
	return nil
}

// NewOnceCloser wraps a close function so it runs at most once.
func NewOnceCloser(closeFn func()) io.Closer {
	return &onceCloser{closeFn: closeFn}
}

type onceCloser struct {
	once    sync.Once
	closeFn func()
}

func (c *onceCloser) Close() error {
	c.once.Do(func() {
		if c.closeFn != nil {
			c.closeFn()
		}
	})
	return nil
}

// MultiCloser closes a group of resources and returns the first close error.
type MultiCloser []io.Closer

func (m MultiCloser) Close() error {
	var firstErr error
	for _, closer := range m {
		if closer == nil {
			continue
		}
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

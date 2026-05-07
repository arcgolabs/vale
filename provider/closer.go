package provider

import (
	"io"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
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
type MultiCloser struct {
	closers *collectionlist.List[io.Closer]
}

func NewMultiCloser(closers *collectionlist.List[io.Closer]) MultiCloser {
	if closers == nil {
		closers = collectionlist.NewList[io.Closer]()
	}
	return MultiCloser{closers: closers}
}

func NewMultiCloserFrom(closers ...io.Closer) MultiCloser {
	return NewMultiCloser(collectionlist.NewList(closers...))
}

func (m MultiCloser) Close() error {
	var firstErr error
	m.closers.Range(func(_ int, closer io.Closer) bool {
		if closer == nil {
			return true
		}
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		return true
	})
	return firstErr
}

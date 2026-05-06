package provider

import "io"

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

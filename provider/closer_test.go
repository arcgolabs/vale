package provider

import (
	"errors"
	"testing"
)

type testCloser struct {
	closed *int
	err    error
}

func (c testCloser) Close() error {
	*c.closed = *c.closed + 1
	return c.err
}

func TestMultiCloserClose(t *testing.T) {
	firstErr := errors.New("first")
	secondErr := errors.New("second")
	closed := 0

	err := MultiCloser{
		testCloser{closed: &closed, err: firstErr},
		nil,
		testCloser{closed: &closed, err: secondErr},
		testCloser{closed: &closed},
	}.Close()

	if !errors.Is(err, firstErr) {
		t.Fatalf("expected first error, got %v", err)
	}
	if closed != 3 {
		t.Fatalf("expected 3 closed resources, got %d", closed)
	}
}

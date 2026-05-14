package provider

import (
	"io"
	"sync"
)

// StateStore stores a value with concurrent-safe access and watcher fan-out.
type StateStore[T any] struct {
	mu       sync.RWMutex
	value    T
	watchHub *WatchHub
	snapshot func(T) T
}

// NewStateStore creates a value holder with an optional snapshot function.
// If snapshot is nil, the stored value is returned as-is.
func NewStateStore[T any](value T, snapshot func(T) T) *StateStore[T] {
	return &StateStore[T]{
		value:    value,
		watchHub: NewWatchHub(),
		snapshot: snapshot,
	}
}

// Load returns the current value. If snapshot is configured, it returns a copy.
func (s *StateStore[T]) Load() T {
	if s == nil {
		var zero T
		return zero
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snapshot == nil {
		return s.value
	}
	return s.snapshot(s.value)
}

// Update replaces the current value and notifies watchers.
func (s *StateStore[T]) Update(value T) {
	s.update(value, true)
}

// Set replaces the current value without triggering watchers.
func (s *StateStore[T]) Set(value T) {
	s.update(value, false)
}

func (s *StateStore[T]) update(value T, notify bool) {
	if s == nil {
		return
	}

	s.mu.Lock()
	s.value = value
	s.mu.Unlock()

	if !notify {
		return
	}

	s.watchHub.Notify()
}

// Watch subscribes to future updates.
func (s *StateStore[T]) Watch(onReload func()) io.Closer {
	if s == nil || s.watchHub == nil {
		return NopCloser{}
	}
	return s.watchHub.Watch(onReload)
}

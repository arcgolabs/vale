package provider

import (
	"io"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type WatchHub struct {
	mu        sync.Mutex
	listeners *mapping.Map[int, func()]
	nextID    int
}

func NewWatchHub() *WatchHub {
	return &WatchHub{listeners: mapping.NewMap[int, func()]()}
}

func (h *WatchHub) Watch(onReload func()) io.Closer {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.listeners == nil {
		h.listeners = mapping.NewMap[int, func()]()
	}
	id := h.nextID
	h.nextID++
	h.listeners.Set(id, onReload)
	return NewOnceCloser(func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.listeners.Delete(id)
	})
}

func (h *WatchHub) Notify() {
	listeners := h.snapshot()
	listeners.Range(func(_ int, listener func()) bool {
		if listener != nil {
			listener()
		}
		return true
	})
}

func (h *WatchHub) snapshot() *collectionlist.List[func()] {
	if h == nil {
		return collectionlist.NewList[func()]()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.listeners == nil {
		return collectionlist.NewList[func()]()
	}
	listeners := collectionlist.NewListWithCapacity[func()](h.listeners.Len())
	h.listeners.Range(func(_ int, listener func()) bool {
		listeners.Add(listener)
		return true
	})
	return listeners
}

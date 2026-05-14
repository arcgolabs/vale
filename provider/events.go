package provider

import (
	"time"

	"github.com/arcgolabs/eventx"
)

type Event = eventx.Event
type EventBus = eventx.BusRuntime

const (
	EventNameConfigSourceLoaded    = "provider.config_source.loaded"
	EventNameConfigSourceFailed    = "provider.config_source.failed"
	EventNameSnapshotRecompiled    = "provider.snapshot.recompiled"
	EventNameSnapshotUnchanged     = "provider.snapshot.unchanged"
	EventNameWatchSetupFailed      = "provider.watch.setup_failed"
	EventNameConfigSourceChanged   = "provider.config_source.changed"
	EventNameConfigSourceDebounced = "provider.config_source.debounced"
)

type ConfigSourceLoadedEvent struct {
	Source     string
	Duration   time.Duration
	ConfigSize int
}

func (e ConfigSourceLoadedEvent) Name() string { return EventNameConfigSourceLoaded }

type ConfigSourceFailedEvent struct {
	Source string
	Error  string
}

func (e ConfigSourceFailedEvent) Name() string { return EventNameConfigSourceFailed }

type SnapshotRecompiledEvent struct {
	SourceCount  int
	RouteCount   int
	ServiceCount int
	Fingerprint  string
}

func (e SnapshotRecompiledEvent) Name() string { return EventNameSnapshotRecompiled }

type SnapshotUnchangedEvent struct {
	Source      string
	Fingerprint string
}

func (e SnapshotUnchangedEvent) Name() string { return EventNameSnapshotUnchanged }

type WatchSetupFailedEvent struct {
	Source string
	Error  string
}

func (e WatchSetupFailedEvent) Name() string { return EventNameWatchSetupFailed }

type ConfigSourceChangedEvent struct {
	Source string
}

func (e ConfigSourceChangedEvent) Name() string { return EventNameConfigSourceChanged }

type ConfigSourceDebouncedEvent struct {
	Source       string
	DebounceTime time.Duration
	SourceCount  int
}

func (e ConfigSourceDebouncedEvent) Name() string { return EventNameConfigSourceDebounced }

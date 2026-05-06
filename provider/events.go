package provider

import (
	"context"
	"time"
)

type Event interface {
	Name() string
}

type EventBus interface {
	Publish(context.Context, Event) error
	Close() error
}

const (
	EventNameConfigSourceLoaded  = "provider.config_source.loaded"
	EventNameConfigSourceFailed  = "provider.config_source.failed"
	EventNameSnapshotRecompiled  = "provider.snapshot.recompiled"
	EventNameWatchSetupFailed    = "provider.watch.setup_failed"
	EventNameConfigSourceChanged = "provider.config_source.changed"
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
}

func (e SnapshotRecompiledEvent) Name() string { return EventNameSnapshotRecompiled }

type WatchSetupFailedEvent struct {
	Source string
	Error  string
}

func (e WatchSetupFailedEvent) Name() string { return EventNameWatchSetupFailed }

type ConfigSourceChangedEvent struct {
	Source string
}

func (e ConfigSourceChangedEvent) Name() string { return EventNameConfigSourceChanged }

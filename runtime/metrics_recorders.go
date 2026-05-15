package runtime

import "time"

type SnapshotMetricsRecorder interface {
	ObserveSnapshot(snapshot *CompiledSnapshot)
}

type ReloadMetricsRecorder interface {
	ObserveReload(result string)
}

type ReloadDurationMetricsRecorder interface {
	ObserveReloadDuration(result string, duration time.Duration)
}

type ReloadDebounceMetricsRecorder interface {
	ObserveReloadDebounce(delay time.Duration, sourceCount int)
}

type HealthMetricsRecorder interface {
	ObserveHealth(endpoint *EndpointRuntime, healthy bool)
}

type HealthCheckMetricsRecorder interface {
	ObserveHealthCheck(endpoint *EndpointRuntime, healthy bool, duration time.Duration)
}

type RouteCacheMetricsRecorder interface {
	ObserveRouteCache(hit bool)
}

type RaftApplyMetricsRecorder interface {
	ObserveRaftApply(group string, duration time.Duration, result string)
}

func (g *Gateway) ObserveSnapshot(snapshot *CompiledSnapshot) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(SnapshotMetricsRecorder); ok {
		recorder.ObserveSnapshot(snapshot)
	}
}

func (g *Gateway) ObserveReload(result string) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(ReloadMetricsRecorder); ok {
		recorder.ObserveReload(result)
	}
}

func (g *Gateway) ObserveReloadDuration(result string, duration time.Duration) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(ReloadDurationMetricsRecorder); ok {
		recorder.ObserveReloadDuration(result, duration)
	}
}

func (g *Gateway) ObserveReloadDebounce(delay time.Duration, sourceCount int) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(ReloadDebounceMetricsRecorder); ok {
		recorder.ObserveReloadDebounce(delay, sourceCount)
	}
}

func (g *Gateway) ObserveHealth(endpoint *EndpointRuntime, healthy bool) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(HealthMetricsRecorder); ok {
		recorder.ObserveHealth(endpoint, healthy)
	}
}

func (g *Gateway) ObserveHealthCheck(endpoint *EndpointRuntime, healthy bool, duration time.Duration) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(HealthCheckMetricsRecorder); ok {
		recorder.ObserveHealthCheck(endpoint, healthy, duration)
	}
}

func (g *Gateway) ObserveRouteCache(hit bool) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(RouteCacheMetricsRecorder); ok {
		recorder.ObserveRouteCache(hit)
	}
}

func (g *Gateway) ObserveRaftApply(group string, duration time.Duration, result string) {
	if g == nil {
		return
	}
	if recorder, ok := g.metrics.(RaftApplyMetricsRecorder); ok {
		recorder.ObserveRaftApply(group, duration, result)
	}
}

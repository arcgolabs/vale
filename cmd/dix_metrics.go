package main

import (
	"context"
	"strconv"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/observabilityx"
)

type valedDixMetrics struct {
	events             observabilityx.Counter
	eventDuration      observabilityx.Histogram
	providers          observabilityx.Counter
	providerDuration   observabilityx.Histogram
	resolves           observabilityx.Counter
	resolveDuration    observabilityx.Histogram
	hooks              observabilityx.Counter
	hookDuration       observabilityx.Histogram
	stateTransitions   observabilityx.Counter
	healthChecks       observabilityx.Counter
	healthCheckLatency observabilityx.Histogram
}

var (
	_ dix.Observer              = (*valedDixMetrics)(nil)
	_ dix.ProviderObserver      = (*valedDixMetrics)(nil)
	_ dix.ResolveObserver       = (*valedDixMetrics)(nil)
	_ dix.LifecycleHookObserver = (*valedDixMetrics)(nil)
)

func provideDixObserver(obs observabilityx.Observability) dix.Observer {
	return newValedDixMetrics(obs)
}

func newValedDixMetrics(obs observabilityx.Observability) *valedDixMetrics {
	if obs == nil {
		obs = observabilityx.Nop()
	}
	return &valedDixMetrics{
		events: obs.Counter(observabilityx.NewCounterSpec("dix_events_total",
			observabilityx.WithDescription("Total dix framework events observed by valed."),
			observabilityx.WithLabelKeys("event", "result"),
		)),
		eventDuration: obs.Histogram(observabilityx.NewHistogramSpec("dix_event_duration_seconds",
			observabilityx.WithDescription("Dix framework event duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("event", "result"),
		)),
		providers: obs.Counter(observabilityx.NewCounterSpec("dix_provider_events_total",
			observabilityx.WithDescription("Total dix provider diagnostic events observed by valed."),
			observabilityx.WithLabelKeys("module", "operation", "result"),
		)),
		providerDuration: obs.Histogram(observabilityx.NewHistogramSpec("dix_provider_duration_seconds",
			observabilityx.WithDescription("Dix provider diagnostic duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("module", "operation", "result"),
		)),
		resolves: obs.Counter(observabilityx.NewCounterSpec("dix_resolve_events_total",
			observabilityx.WithDescription("Total dix resolve diagnostic events observed by valed."),
			observabilityx.WithLabelKeys("operation", "result"),
		)),
		resolveDuration: obs.Histogram(observabilityx.NewHistogramSpec("dix_resolve_duration_seconds",
			observabilityx.WithDescription("Dix resolve diagnostic duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("operation", "result"),
		)),
		hooks: obs.Counter(observabilityx.NewCounterSpec("dix_lifecycle_hook_events_total",
			observabilityx.WithDescription("Total dix lifecycle hook diagnostic events observed by valed."),
			observabilityx.WithLabelKeys("kind", "name", "result"),
		)),
		hookDuration: obs.Histogram(observabilityx.NewHistogramSpec("dix_lifecycle_hook_duration_seconds",
			observabilityx.WithDescription("Dix lifecycle hook diagnostic duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("kind", "name", "result"),
		)),
		stateTransitions: obs.Counter(observabilityx.NewCounterSpec("dix_state_transitions_total",
			observabilityx.WithDescription("Total dix runtime state transitions observed by valed."),
			observabilityx.WithLabelKeys("from", "to", "reason"),
		)),
		healthChecks: obs.Counter(observabilityx.NewCounterSpec("dix_health_checks_total",
			observabilityx.WithDescription("Total dix framework health checks observed by valed."),
			observabilityx.WithLabelKeys("kind", "name", "result"),
		)),
		healthCheckLatency: obs.Histogram(observabilityx.NewHistogramSpec("dix_health_check_duration_seconds",
			observabilityx.WithDescription("Dix framework health check duration in seconds."),
			observabilityx.WithUnit("s"),
			observabilityx.WithLabelKeys("kind", "name", "result"),
		)),
	}
}

func (m *valedDixMetrics) OnBuild(ctx context.Context, event dix.BuildEvent) {
	m.observeEvent(ctx, "build", eventResult(event.Err), event.Duration.Seconds())
}

func (m *valedDixMetrics) OnStart(ctx context.Context, event dix.StartEvent) {
	m.observeEvent(ctx, "start", eventResult(event.Err), event.Duration.Seconds())
}

func (m *valedDixMetrics) OnStop(ctx context.Context, event dix.StopEvent) {
	m.observeEvent(ctx, "stop", eventResult(event.Err), event.Duration.Seconds())
}

func (m *valedDixMetrics) OnHealthCheck(ctx context.Context, event dix.HealthCheckEvent) {
	result := eventResult(event.Err)
	attrs := []observabilityx.Attribute{
		observabilityx.String("kind", string(event.Kind)),
		observabilityx.String("name", event.Name),
		observabilityx.String("result", result),
	}
	m.healthChecks.Add(metricContext(ctx), 1, attrs...)
	m.healthCheckLatency.Record(metricContext(ctx), event.Duration.Seconds(), attrs...)
}

func (m *valedDixMetrics) OnStateTransition(ctx context.Context, event dix.StateTransitionEvent) {
	m.stateTransitions.Add(metricContext(ctx), 1,
		observabilityx.String("from", event.From.String()),
		observabilityx.String("to", event.To.String()),
		observabilityx.String("reason", event.Reason),
	)
}

func (m *valedDixMetrics) OnProvider(ctx context.Context, event dix.ProviderEvent) {
	result := eventResult(event.Err)
	attrs := []observabilityx.Attribute{
		observabilityx.String("module", event.Module),
		observabilityx.String("operation", event.Operation),
		observabilityx.String("result", result),
	}
	m.providers.Add(metricContext(ctx), 1, attrs...)
	m.providerDuration.Record(metricContext(ctx), event.Duration.Seconds(), attrs...)
}

func (m *valedDixMetrics) OnResolve(ctx context.Context, event dix.ResolveEvent) {
	result := eventResult(event.Err)
	attrs := []observabilityx.Attribute{
		observabilityx.String("operation", event.Operation),
		observabilityx.String("result", result),
	}
	m.resolves.Add(metricContext(ctx), 1, attrs...)
	m.resolveDuration.Record(metricContext(ctx), event.Duration.Seconds(), attrs...)
}

func (m *valedDixMetrics) OnLifecycleHook(ctx context.Context, event dix.LifecycleHookEvent) {
	result := eventResult(event.Err)
	attrs := []observabilityx.Attribute{
		observabilityx.String("kind", string(event.Kind)),
		observabilityx.String("name", lifecycleHookName(event)),
		observabilityx.String("result", result),
	}
	m.hooks.Add(metricContext(ctx), 1, attrs...)
	m.hookDuration.Record(metricContext(ctx), event.Duration.Seconds(), attrs...)
}

func (m *valedDixMetrics) observeEvent(ctx context.Context, event, result string, duration float64) {
	attrs := []observabilityx.Attribute{
		observabilityx.String("event", event),
		observabilityx.String("result", result),
	}
	m.events.Add(metricContext(ctx), 1, attrs...)
	m.eventDuration.Record(metricContext(ctx), duration, attrs...)
}

func lifecycleHookName(event dix.LifecycleHookEvent) string {
	if event.Name != "" {
		return event.Name
	}
	if event.Label != "" {
		return event.Label
	}
	return "sequence_" + strconv.Itoa(event.Sequence)
}

func eventResult(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func metricContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

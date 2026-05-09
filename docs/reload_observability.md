# Reload Observability

Vale reloads are provider-driven. File, memory, Docker, and K8s-like providers
feed the same merge, validate, fingerprint, compile, and swap pipeline.

## Admin Endpoint

`GET /admin/reload/status` returns the latest reload state:

- `state`: `loaded`, `swapped`, `restarted`, or `failed`.
- `fingerprint`: stable config fingerprint from the merged config pipeline.
- `last_error`: last reload or watch error.
- `routes` / `services`: active object counts.
- `static_fields`: fields that required server restart during hot reload.
- `diff`: route, service, and endpoint names added, removed, or changed.

## Events

Provider events are still emitted on the configured event bus:

- `provider.config_source.loaded`
- `provider.config_source.failed`
- `provider.config_source.changed`
- `provider.snapshot.recompiled`
- `provider.snapshot.unchanged`
- `provider.watch.setup_failed`
- `gateway.static_runtime_config.changed`

The admin endpoint is the low-friction operational view; the event bus is the
embedding hook for external controllers.

## Raft State

The optional `cluster/raftnode` module keeps the last applied typed FSM state in
Dragonboat. Restarted nodes recover state through Dragonboat log replay and
snapshots; the adapter does not maintain a second bbolt applied-state database
by default.

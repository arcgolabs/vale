# Vela Gateway Prototype (V1 Skeleton)

This repository now contains a runnable `velad` prototype aligned to the technical selection document.

Product and technical specs live under [`docs/`](./docs/README.md) (Chinese).

## Current Capability

- HCL file config (`entrypoint` / `service` / `route` / `admin` / `observability` / `health`)
- Compiled snapshot + atomic hot swap
- Compiled route index: exact-host -> wildcard-host -> path-prefix/method/header predicates
- Round-robin and weighted round-robin endpoint picking
- Built-in reverse proxy engine based on `oxy`
- File-based config watching with invalid-config rollback behavior
- JSON access logging and Prometheus metrics
- Admin API for routes/services/endpoints and `/metrics`
- Control-plane route catalog backed by `go-memdb` for admin queries and future reload diffing
- Active endpoint health checks
- Library-first builders for runtime snapshots and config source assembly
- Provider reload coalescing with stable config fingerprints
- Built-in middleware plus a runtime middleware registry for embedded extensions
- Static TLS and ACME with secure defaults

## Architecture Boundary

- `runtime`: data plane runtime, consumes only compiled snapshot.
- `config`: HCL/JSON DTOs and validation. Native slices/maps are kept here because this is the decoder boundary.
- `compiler`: control-plane compile step from config DTOs to collectionx-backed runtime snapshot.
- `provider/fileconfig`: optional HCL file config source provider (file + watch).
- `provider/merged`: multi-source merge + validate + compile.
- `provider`: config builder helpers for embedded/library-first use.
- root `vela`: library-first public API (`New` / `Start` / `Stop`).
- `gateway`: lower-level embedded gateway implementation.
- `cmd`: `velad` executable (`dix` graph + `configx` + Cobra + run loop).

This keeps runtime dependency-light and moves provider/config concerns outside runtime.

## Workspace

This repository includes a `go.work` file for local development and release builds:

```bash
go work sync
go test ./...
go test ./cluster/raftnode/... ./cmd/... ./observability/prometheus/... ./provider/docker/... ./provider/file/... ./provider/fileconfig/... ./provider/k8s/... ./examples/embedded_multi_provider/... ./examples/embedded_static_config/...
go run ./cmd
```

The workspace should not rely on local `replace` directives in `go.mod`. As the repo grows into multiple modules, local module wiring should live in `go.work`, while each module keeps publishable module paths in its own `go.mod`.

Current workspace modules:

- `github.com/arcgolabs/vela`: library-first core module.
- `github.com/arcgolabs/vela/cmd`: standalone `velad` binary wiring.
- `github.com/arcgolabs/vela/cluster/raftnode`: optional HashiCorp Raft cluster adapter.
- `github.com/arcgolabs/vela/observability/prometheus`: optional Prometheus metrics adapter.
- `github.com/arcgolabs/vela/provider/docker`: optional Docker config provider.
- `github.com/arcgolabs/vela/provider/file`: optional HCL snapshot provider.
- `github.com/arcgolabs/vela/provider/fileconfig`: optional HCL config source provider.
- `github.com/arcgolabs/vela/provider/k8s`: optional K8s-like config provider.
- `github.com/arcgolabs/vela/examples/embedded_multi_provider`: example that consumes optional provider modules.
- `github.com/arcgolabs/vela/examples/embedded_static_config`: example that consumes the core event bus.

Local workspace modules are intentionally not declared as `replace` directives. `go.work`
resolves them during repository development; published modules should use real released
versions when consumed outside this workspace.

## arcgolabs Integration

- `github.com/arcgolabs/dix`: dependency injection for `velad` daemon assembly in `cmd`.
- `github.com/arcgolabs/logx`: structured logger construction and lifecycle.
- `github.com/arcgolabs/configx`: bootstrap config from env/defaults.
- `github.com/arcgolabs/eventx`: core provider load/reload/failure event bus.
- `github.com/arcgolabs/collectionx`: list/set/map abstractions for config assembly, matcher grouping, and validation; prefix trie for path route buckets; bitset for compiled route predicates; graph for config reference validation.
- `github.com/hashicorp/go-memdb`: immutable-radix-backed control-plane catalog for compiled routes/services without replacing the hot-path matcher.

`runtime` package does not depend on DI container, matching the document's "core runtime no DI" rule.

## Run

Process bootstrap is loaded inside the `velad` [dix](https://github.com/arcgolabs/dix) graph via [configx](https://github.com/arcgolabs/configx) (`WithTypedDefaults` → `VELA_*` env → explicit CLI flags on Cobra’s `pflag` set). Flags exist for `--help` and parsing; merge order is configx’s. See `velad --help`.

`velad` can start without a config file. In that case it uses the built-in default config:

- entrypoint `web` on `:8080`
- admin on `:19090`
- one `echo` service pointing at `http://127.0.0.1:8081`
- route `/` to `echo`
- access log and metrics enabled

Start with defaults:

```bash
go run ./cmd
```

To run with an HCL file, copy sample config:

```bash
cp vela.example.hcl vela.hcl
```

Start an upstream service (example):

```bash
python -m http.server 8081
```

Start gateway with an explicit config:

```bash
go run ./cmd -config ./vela.hcl
```

Or merge multiple files (later files override same-name objects):

```bash
go run ./cmd -config-files "./base.hcl,./service.hcl,./override.hcl"
```

The reverse proxy engine is built in and uses `oxy`.

TLS and ACME defaults:

- TLS listeners use Go's secure TLS defaults with minimum TLS 1.2.
- ACME uses `golang.org/x/crypto/acme/autocert`.
- When ACME is enabled and `cache_dir` is omitted, Vela uses `.vela/acme`.
- ACME config requires explicit domains and email in file config.

Verify:

- `http://127.0.0.1:8080/`
- `http://127.0.0.1:19090/metrics`
- `http://127.0.0.1:19090/admin/routes`
- `http://127.0.0.1:19090/admin/routes?service=echo`
- `http://127.0.0.1:19090/admin/services`
- `http://127.0.0.1:19090/admin/endpoints`
- `http://127.0.0.1:19090/admin/cluster/status`
- `http://127.0.0.1:19090/admin/cluster/peers`

## Embedded API

Use `github.com/arcgolabs/vela` as the primary library import path.
`vela.New()` uses the built-in default config when no config path, config provider,
snapshot provider, or static config is supplied.

```go
import (
  "context"
  "log/slog"

  "github.com/arcgolabs/vela"
  fileconfig "github.com/arcgolabs/vela/provider/fileconfig"
)

func runEmbedded() error {
  g, err := vela.New(
    fileconfig.WithConfigPath("./vela.hcl"),
    vela.WithWatch(true),
    vela.WithLogger(slog.Default()),
  )
  if err != nil {
    return err
  }

  if err := g.Start(context.Background()); err != nil {
    return err
  }
  // ... your app lifecycle ...
  return g.Stop(context.Background())
}
```

`vela.NewFromConfig(vela.Config{...})` is also available for struct-based construction.

For code-first runtime construction, the root `vela` package exposes
collectionx-backed helpers:

```go
endpoint, _ := vela.NewEndpoint("http://127.0.0.1:8081", 1, http.DefaultServeMux)
service := vela.NewService("api", "round_robin", endpoint)
route := vela.NewRoute("api", "web", service).WithPathPrefix("/api")

snapshot := vela.NewSnapshot().
  AddEntrypoint("web", ":8080", vela.RuntimeEntrypoint{}).
  AddService(service).
  AddRoute(route).
  BuildMatchers()
```

For config-first construction without HCL, use `vela.NewConfigBuilder()` and pass
the result to `vela.WithStaticConfig`:

```go
cfg := vela.NewConfigBuilder().
  Entrypoint("web", ":8080").
  Service("api", "http://127.0.0.1:8081").
  MiddlewareNamed("strip-api", vela.MiddlewareStripPrefix("/api")).
  RouteTo("api", "web", "api",
    vela.RoutePathPrefix("/api"),
    vela.RouteMiddlewares("strip-api"),
  ).
  Admin(":19090").
  Observability(true, true).
  Health("5s", "2s").
  Build()
```

Use `BuildValidated()` when constructing config from user input; it returns the
config together with accumulated builder errors plus `config.Validate` errors.

### Embedded Static Config Example

See `examples/embedded_static_config/main.go` for a full example that uses:

- `vela.WithStaticConfig(...)`
- `vela.WithEventBus(...)`
- `eventx.Subscribe(...)` to consume provider lifecycle events

Run:

```bash
go run ./examples/embedded_static_config
```

### Embedded Multi-Provider Example

See `examples/embedded_multi_provider/main.go` for combining providers in memory:

- Docker-like source provider
- K8s-like source provider
- merge pipeline + event bus

Run:

```bash
go run ./examples/embedded_multi_provider
```

Constructor options currently include:

- `fileconfig.WithConfigPath(path)`
- `fileconfig.WithConfigFiles(path1, path2, ...)` (merge order: left -> right, later wins)
- `vela.WithWatch(enabled)`
- `vela.WithClusterFactory(factory)` (optional control-plane cluster adapter)
- `vela.WithLogger(logger)`
- `vela.WithEventBus(bus)` (subscribe provider lifecycle events)
- `vela.WithMetricsFactory(factory)` (optional metrics recorder adapter)
- `vela.WithMiddlewareRegistry(registry)` (embedded runtime middleware extensions)
- `vela.WithSnapshotProvider(provider)` (advanced/custom provider)
- `vela.WithConfigSourceProviders(...)` (advanced merge pipeline input)
- `docker.NewFromEnv(name, options)` for Docker daemon-backed source
- `vela.WithStaticConfig(config)` (inject in-memory config as source, watch off)
- `vela.WithFallbackProviders(p1, p2, ...)` (provider failover chain)
- `vela.WithStaticSnapshot(snapshot)` (in-memory embedded mode)
- `vela.WithWatchErrorHandler(handler)`

Provider events currently emitted on the event bus:

- `provider.config_source.loaded`
- `provider.config_source.failed`
- `provider.config_source.changed`
- `provider.snapshot.recompiled`
- `provider.snapshot.unchanged` (watch event produced no config fingerprint change)
- `provider.watch.setup_failed`
- `gateway.static_runtime_config.changed` (hot-reloaded snapshot changed fields that require process restart)

### Mutable In-Memory Provider (for embedded dynamic updates)

For non-file embedded scenarios, you can use `provider/memoryconfig` with:

- `memoryconfig.New(name, cfg)`
- `provider.Update(newCfg)` to trigger hot reload through the same merge/compile pipeline

Merged providers coalesce rapid watch events and compare stable config fingerprints before
publishing a reload. Unchanged updates publish `provider.snapshot.unchanged` and do not
swap the runtime snapshot.

### Middleware Extensions

Built-in middleware supports path prefix rewriting, request/response headers, and body
limits. Embedded users can register runtime middleware factories:

```go
registry := vela.DefaultMiddlewareRegistry()
_ = registry.Register("custom", func(next http.Handler, middleware vela.RuntimeMiddleware) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    next.ServeHTTP(w, r)
  })
})
g, err := vela.New(vela.WithMiddlewareRegistry(registry))
```

### Metrics

The default `observabilityx` metrics recorder exposes:

- `vela_http_requests_total`
- `vela_http_request_duration_seconds`
- `vela_runtime_reloads_total`
- `vela_health_checks_total`
- `vela_active_routes`
- `vela_active_services`
- `vela_active_endpoints`

### Provider Expansion Notes

- `provider/docker`: optional module, label-driven route/service projection (source pluggable).
- `provider/k8s`: optional module, route/endpoint projection from k8s-like source model (source pluggable).
- both packages include `MemorySource` for local embedding/tests and can be replaced by real API clients later.

### Raft Control-Plane (Experimental)

Standalone flags:

- `--raft-enabled`
- `--raft-node-id`
- `--raft-bind`
- `--raft-data-dir`
- `--raft-bootstrap`

When enabled, `cmd` wires the optional `cluster/raftnode` module into the gateway via
`raftnode.WithCluster(...)`, starts an embedded HashiCorp Raft node, and exposes status at:

- `/admin/cluster/status`
- `/admin/cluster/peers`

Leader-only membership APIs:

- `POST /admin/cluster/join` body: `{"id":"node-2","address":"127.0.0.1:17001"}`
- `POST /admin/cluster/leave` body: `{"id":"node-2"}`

Raft apply payloads are structured commands. The current command stores snapshot
metadata in the FSM as typed JSON so the adapter can evolve toward replicated
config state without changing the gateway cluster interface.

## Bootstrap Env Variables

- `VELA_CONFIG`
- `VELA_CONFIG_FILES` (comma-separated)
- `VELA_WATCH`
- `VELA_LOG_LEVEL`
- `VELA_RAFT_ENABLED`
- `VELA_RAFT_NODE_ID`
- `VELA_RAFT_BIND`
- `VELA_RAFT_DATA_DIR`
- `VELA_RAFT_BOOTSTRAP`

Admin/observability/health runtime knobs are read from the HCL snapshot.

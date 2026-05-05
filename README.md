# Vela Gateway Prototype (V1 Skeleton)

This repository now contains a runnable `velad` prototype aligned to the technical selection document.

Product and technical specs live under [`docs/`](./docs/README.md) (Chinese).

## Current Capability

- HCL file config (`entrypoint` / `service` / `route` / `proxy_engine` / `admin` / `observability` / `health`)
- Compiled snapshot + atomic hot swap
- Compiled route index: exact-host -> wildcard-host -> path-prefix/method/header predicates
- Round-robin and weighted round-robin endpoint picking
- Reverse proxy engine selectable by config: `stdlib` (default) or `oxy`
- File-based config watching with invalid-config rollback behavior
- JSON access logging and Prometheus metrics
- Admin API for routes/services/endpoints and `/metrics`
- Active endpoint health checks

## Architecture Boundary

- `runtime`: data plane runtime, consumes only compiled snapshot.
- `compiler`: control-plane compile step from config to runtime snapshot.
- `provider/fileconfig`: config source provider (file + watch).
- `provider/merged`: multi-source merge + validate + compile.
- root `vela`: library-first public API (`New` / `Start` / `Stop`).
- `gateway`: lower-level embedded gateway implementation.
- `cmd`: `velad` executable (`dix` graph + `configx` + Cobra + run loop).

This keeps runtime dependency-light and moves provider/config concerns outside runtime.

## Workspace

This repository includes a `go.work` file for local development and release builds:

```bash
go work sync
go test ./...
go run ./cmd
```

The workspace should not rely on local `replace` directives in `go.mod`. As the repo grows into multiple modules, local module wiring should live in `go.work`, while each module keeps publishable module paths in its own `go.mod`.

## arcgolabs Integration

- `github.com/arcgolabs/dix`: dependency injection for `velad` daemon assembly in `cmd`.
- `github.com/arcgolabs/logx`: structured logger construction and lifecycle.
- `github.com/arcgolabs/configx`: bootstrap config from env/defaults.
- `github.com/arcgolabs/eventx`: provider load/reload/failure event bus.
- `github.com/arcgolabs/collectionx`: ordered config source registry for merge pipeline.

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

You can switch proxy engine in HCL:

```hcl
proxy_engine = "stdlib" # or "oxy"
```

Verify:

- `http://127.0.0.1:8080/`
- `http://127.0.0.1:19090/metrics`
- `http://127.0.0.1:19090/admin/routes`
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
)

func runEmbedded() error {
  g, err := vela.New(
    vela.WithConfigPath("./vela.hcl"),
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

- `vela.WithConfigPath(path)`
- `vela.WithConfigFiles(path1, path2, ...)` (merge order: left -> right, later wins)
- `vela.WithWatch(enabled)`
- `vela.WithRaftCluster(config)` (optional raft control-plane node)
- `vela.WithLogger(logger)`
- `vela.WithEventBus(bus)` (subscribe provider lifecycle events)
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
- `provider.watch.setup_failed`
- `gateway.static_runtime_config.changed` (hot-reloaded snapshot changed fields that require process restart)

### Mutable In-Memory Provider (for embedded dynamic updates)

For non-file embedded scenarios, you can use `provider/memoryconfig` with:

- `memoryconfig.New(name, cfg)`
- `provider.Update(newCfg)` to trigger hot reload through the same merge/compile pipeline

### Provider Expansion Notes

- `provider/docker`: label-driven route/service projection (source pluggable).
- `provider/k8s`: route/endpoint projection from k8s-like source model (source pluggable).
- both packages include `MemorySource` for local embedding/tests and can be replaced by real API clients later.

### Raft Control-Plane (Experimental)

Standalone flags:

- `--raft-enabled`
- `--raft-node-id`
- `--raft-bind`
- `--raft-data-dir`
- `--raft-bootstrap`

When enabled, gateway starts an embedded HashiCorp Raft node and exposes status at:

- `/admin/cluster/status`
- `/admin/cluster/peers`

Leader-only membership APIs:

- `POST /admin/cluster/join` body: `{"id":"node-2","address":"127.0.0.1:17001"}`
- `POST /admin/cluster/leave` body: `{"id":"node-2"}`

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

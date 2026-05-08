# Vale Gateway

Vale is a library-first Go reverse proxy gateway that can also be packaged as
the standalone `valed` binary. The root module is the preferred embedded API,
while optional workspace modules provide integrations such as file config,
Docker labels, K8s-like sources, Prometheus metrics, and Raft control-plane
state.

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
- Control-plane route catalog backed by `go-memdb` for admin queries and reload diffing
- Active endpoint health checks
- Library-first builders for runtime snapshots and config source assembly
- Provider reload coalescing with stable config fingerprints
- Built-in middleware plus a runtime middleware registry for embedded extensions
- Built-in basic auth, gzip compression, IP allow list, CORS, rate limit, circuit breaker, security headers, path/header/redirect policies
- Static TLS and ACME with secure defaults

## Status

The project has published `v0.1.0`. The public import path follows the current
git remote: `github.com/arcgolabs/vale`.

## Architecture Boundary

- `runtime`: data plane runtime, consumes only compiled snapshot.
- `config`: HCL/JSON DTOs and validation. Native slices/maps are kept here because this is the decoder boundary.
- `compiler`: control-plane compile step from config DTOs to collectionx-backed runtime snapshot.
- `provider/fileconfig`: optional HCL file config source provider (file + watch).
- `provider/merged`: multi-source merge + validate + compile.
- `provider`: config builder helpers for embedded/library-first use.
- root `vale`: library-first public API (`New` / `Start` / `Stop`).
- `gateway`: lower-level embedded gateway implementation.
- `cmd`: `valed` executable (`dix` graph + `configx` + Cobra + run loop).

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

- `github.com/arcgolabs/vale`: library-first core module.
- `github.com/arcgolabs/vale/cmd`: standalone `valed` binary wiring.
- `github.com/arcgolabs/vale/cluster/raftnode`: optional HashiCorp Raft cluster adapter.
- `github.com/arcgolabs/vale/observability/prometheus`: optional Prometheus metrics adapter.
- `github.com/arcgolabs/vale/provider/docker`: optional Docker config provider.
- `github.com/arcgolabs/vale/provider/file`: optional HCL snapshot provider.
- `github.com/arcgolabs/vale/provider/fileconfig`: optional HCL config source provider.
- `github.com/arcgolabs/vale/provider/k8s`: optional K8s-like config provider.
- `github.com/arcgolabs/vale/examples/embedded_multi_provider`: example that consumes optional provider modules.
- `github.com/arcgolabs/vale/examples/embedded_static_config`: example that consumes the core event bus.

Local workspace modules are intentionally not declared as `replace` directives. `go.work`
resolves them during repository development; published modules should use real released
versions when consumed outside this workspace.

## arcgolabs Integration

- `github.com/arcgolabs/dix`: dependency injection for `valed` daemon assembly in `cmd`.
- `github.com/arcgolabs/logx`: structured logger construction and lifecycle.
- `github.com/arcgolabs/configx`: bootstrap config from env/defaults.
- `github.com/arcgolabs/eventx`: core provider load/reload/failure event bus.
- `github.com/arcgolabs/collectionx`: list/set/map abstractions for config assembly, matcher grouping, and validation; prefix trie for path route buckets; bitset for compiled route predicates; graph for config reference validation.
- `github.com/hashicorp/go-memdb`: immutable-radix-backed control-plane catalog for compiled routes/services without replacing the hot-path matcher.

`runtime` package does not depend on DI container, matching the document's "core runtime no DI" rule.

## Run

Process bootstrap is loaded inside the `valed` [dix](https://github.com/arcgolabs/dix) graph via [configx](https://github.com/arcgolabs/configx) (`WithTypedDefaults` → `VALE_*` env → explicit CLI flags on Cobra’s `pflag` set). Flags exist for `--help` and parsing; merge order is configx’s. See `valed --help`.

`valed` can start without a config file. In that case it uses the built-in default config:

- entrypoint `web` on `:8080`
- admin on `:19090`
- one `echo` service pointing at `http://127.0.0.1:8081`
- route `/` to `echo`
- access log and metrics enabled

Start with defaults:

```bash
go run ./cmd
```

Run the published container image:

```bash
docker run --rm -p 8080:8080 -p 19090:19090 ghcr.io/arcgolabs/vale:v0.1.0
```

To run with an HCL file, copy sample config:

```bash
cp vale.example.hcl vale.hcl
```

Start an upstream service (example):

```bash
python -m http.server 8081
```

Start gateway with an explicit config:

```bash
go run ./cmd -config ./vale.hcl
```

Or merge multiple files (later files override same-name objects):

```bash
go run ./cmd -config-files "./base.hcl,./service.hcl,./override.hcl"
```

The reverse proxy engine is built in and uses `oxy`.

TLS and ACME defaults:

- TLS listeners use Go's secure TLS defaults with minimum TLS 1.2.
- ACME uses `golang.org/x/crypto/acme/autocert`.
- When ACME is enabled and `cache_dir` is omitted, Vale uses `.vale/acme`.
- ACME config requires explicit domains and email in file config.

Verify:

- `http://127.0.0.1:8080/`
- `http://127.0.0.1:19090/metrics`
- `http://127.0.0.1:19090/admin/routes`
- `http://127.0.0.1:19090/admin/routes?service=echo`
- `http://127.0.0.1:19090/admin/services`
- `http://127.0.0.1:19090/admin/endpoints`
- `http://127.0.0.1:19090/admin/reload/status`
- `http://127.0.0.1:19090/admin/cluster/status`
- `http://127.0.0.1:19090/admin/cluster/peers`

## Embedded API

Use `github.com/arcgolabs/vale` as the primary library import path.
`vale.New()` uses the built-in default config when no config path, config provider,
snapshot provider, or static config is supplied.

```go
import (
  "context"
  "log/slog"

  "github.com/arcgolabs/vale"
  fileconfig "github.com/arcgolabs/vale/provider/fileconfig"
)

func runEmbedded() error {
  g, err := vale.New(
    fileconfig.WithConfigPath("./vale.hcl"),
    vale.WithWatch(true),
    vale.WithLogger(slog.Default()),
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

`vale.NewFromConfig(vale.Config{...})` is also available for struct-based construction.

For code-first runtime construction, the root `vale` package exposes
collectionx-backed helpers:

```go
endpoint, _ := vale.NewEndpoint("http://127.0.0.1:8081", 1, http.DefaultServeMux)
service := vale.NewService("api", "round_robin", endpoint)
route := vale.NewRoute("api", "web", service).WithPathPrefix("/api")

snapshot := vale.NewSnapshot().
  AddEntrypoint("web", ":8080", vale.RuntimeEntrypoint{}).
  AddService(service).
  AddRoute(route).
  BuildMatchers()
```

For config-first construction without HCL, use `vale.NewConfigBuilder()` and pass
the result to `vale.WithStaticConfig`:

```go
cfg := vale.NewConfigBuilder().
  Entrypoint("web", ":8080").
  Service("api", "http://127.0.0.1:8081").
  MiddlewareNamed("strip-api", vale.MiddlewareStripPrefix("/api")).
  RouteTo("api", "web", "api",
    vale.RoutePathPrefix("/api"),
    vale.RouteMiddlewares("strip-api"),
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

- `vale.WithStaticConfig(...)`
- `vale.WithEventBus(...)`
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
- `vale.WithWatch(enabled)`
- `vale.WithClusterFactory(factory)` (optional control-plane cluster adapter)
- `vale.WithLogger(logger)`
- `vale.WithEventBus(bus)` (subscribe provider lifecycle events)
- `vale.WithMetricsFactory(factory)` (optional metrics recorder adapter)
- `vale.WithMiddlewareRegistry(registry)` (embedded runtime middleware extensions)
- `vale.WithSnapshotProvider(provider)` (advanced/custom provider)
- `vale.WithConfigSourceProviders(...)` (advanced merge pipeline input)
- `docker.NewFromEnv(name, options)` for Docker daemon-backed source
- `vale.WithStaticConfig(config)` (inject in-memory config as source, watch off)
- `vale.WithFallbackProviders(p1, p2, ...)` (provider failover chain)
- `vale.WithStaticSnapshot(snapshot)` (in-memory embedded mode)
- `vale.WithWatchErrorHandler(handler)`

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

Reload state is exposed at `/admin/reload/status` with the latest state,
fingerprint, error, route/service/endpoint diff, and static fields that required
a server restart.

### Middleware Extensions

Built-in middleware supports:

- path transforms: `strip_prefix`, `add_prefix`, `replace_path`, `replace_path_regex`
- redirects: `redirect_scheme`, `redirect_regex`
- headers and secure headers
- CORS
- rate limit
- circuit breaker
- basic auth
- gzip compression
- IP allow list
- request body limits
- middleware chains

Embedded users can register runtime middleware factories:

```go
registry := vale.DefaultMiddlewareRegistry()
_ = registry.Register("custom", func(next http.Handler, middleware vale.RuntimeMiddleware) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    next.ServeHTTP(w, r)
  })
})
g, err := vale.New(vale.WithMiddlewareRegistry(registry))
```

### Metrics

The default `observabilityx` metrics recorder exposes:

- `vale_http_requests_total`
- `vale_http_request_duration_seconds`
- `vale_runtime_reloads_total`
- `vale_health_checks_total`
- `vale_active_routes`
- `vale_active_services`
- `vale_active_endpoints`

### Provider Expansion Notes

- `provider/docker`: optional module, label-driven route/service projection (source pluggable).
  It accepts native `vale.*` labels and a Traefik-compatible HTTP label subset:
  `traefik.enable`, `traefik.http.routers.*.rule`, `entrypoints`, `middlewares`,
  `service`, `traefik.http.services.*.loadbalancer.server.port/scheme`, and
  `addPrefix`, `stripPrefix`, `replacePath`, `redirectScheme`, `redirectRegex`,
  chain, headers, CORS headers, buffering, basicAuth, compress, ipAllowList,
  ipWhiteList, and rateLimit middleware labels.
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

Raft apply payloads are structured commands. Route sync commands store snapshot
metadata and route records in the FSM as typed JSON and persist the applied
state through `github.com/arcgolabs/storx/bboltx`, so the adapter can evolve
toward replicated config state without changing the gateway cluster interface.
Restarted nodes load the last applied FSM state from the bboltx state store
before Raft replay catches up.

## Bootstrap Env Variables

- `VALE_CONFIG`
- `VALE_CONFIG_FILES` (comma-separated)
- `VALE_WATCH`
- `VALE_LOG_LEVEL`
- `VALE_RAFT_ENABLED`
- `VALE_RAFT_NODE_ID`
- `VALE_RAFT_BIND`
- `VALE_RAFT_DATA_DIR`
- `VALE_RAFT_BOOTSTRAP`

Admin/observability/health runtime knobs are read from the HCL snapshot.

## Container Images

Release workflow publishes multi-arch Linux images to GHCR:

- `ghcr.io/arcgolabs/vale:<tag>`
- `ghcr.io/arcgolabs/vale:<semver-without-v>`
- `ghcr.io/arcgolabs/vale:latest` for non-prerelease tags

For example:

```bash
docker run --rm -p 8080:8080 -p 19090:19090 ghcr.io/arcgolabs/vale:v0.1.0
```

## License

MIT. See [LICENSE](./LICENSE).

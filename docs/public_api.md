# Public API Matrix

This repository is library-first. The root package is the preferred import path for
embedded users; subpackages remain available for advanced wiring and optional modules.

## Core Module

| Package | Stability target | Purpose |
| --- | --- | --- |
| `github.com/arcgolabs/vale` | Public | Primary embedded API, functional options, runtime builders, config builders. |
| `github.com/arcgolabs/vale/config` | Public DTO | HCL/JSON config structs, validation, fingerprinting. Native slices/maps remain here for decoder compatibility. |
| `github.com/arcgolabs/vale/runtime` | Public advanced | Compiled data-plane model, matcher, control-plane catalog, middleware registry, metrics contracts. Uses collectionx containers. |
| `github.com/arcgolabs/vale/provider` | Public advanced | Provider interfaces, config builder implementation, events, fallback provider, closers. |
| `github.com/arcgolabs/vale/provider/merged` | Public advanced | Multi-source merge, validate, compile, debounce, fingerprint dedupe. |
| `github.com/arcgolabs/vale/provider/memoryconfig` | Public advanced | Mutable in-memory config source for embedded dynamic updates. |
| `github.com/arcgolabs/vale/provider/static` | Public advanced | Static compiled snapshot provider. |
| `github.com/arcgolabs/vale/provider/staticconfig` | Public advanced | Static config DTO provider. |
| `github.com/arcgolabs/vale/compiler` | Advanced | Config-to-runtime compiler. Useful for custom provider pipelines. |
| `github.com/arcgolabs/vale/gateway` | Advanced | Lower-level gateway orchestration used by root package. |
| `github.com/arcgolabs/vale/proxy` | Internal-leaning | Built-in oxy proxy construction. Prefer root/runtime APIs unless customizing internals. |

## Optional Modules

| Module | Purpose |
| --- | --- |
| `github.com/arcgolabs/vale/cmd` | Standalone `valed` binary. |
| `github.com/arcgolabs/vale/cluster/raftnode` | Optional Dragonboat multi-group Raft control-plane adapter. |
| `github.com/arcgolabs/vale/observability/prometheus` | Optional Prometheus adapter for metrics exposition. |
| `github.com/arcgolabs/vale/provider/docker` | Optional Docker label-driven config source. |
| `github.com/arcgolabs/vale/provider/file` | Optional HCL snapshot provider. |
| `github.com/arcgolabs/vale/provider/fileconfig` | Optional HCL config source provider. |
| `github.com/arcgolabs/vale/provider/k8s` | Optional K8s-like config source. |
| `github.com/arcgolabs/vale/examples/*` | Example consumers, not API. |

## Compatibility Notes

- The root package should remain the stable surface for typical embedded usage.
- Extensions are compile-time library composition points, not runtime plugins. Embedded
  users should import Vale and register custom providers, middleware, certificate
  storage, cluster factories, metrics, or observability factories through `vale.Registry`.
- `config` DTO structs intentionally use native Go collections because HCL/JSON decoding is their boundary.
- Compiled runtime structs intentionally use `collectionx` containers.
- Runtime route catalog queries return collectionx lists and keep request matching on the optimized matcher.
- Runtime snapshot diff helpers return collectionx-backed route/service/endpoint change sets for reload observability.
- Admin HTTP responses are stable plain JSON DTOs and should not expose collectionx serialization details.
- Repository-local sibling modules are resolved by `go.work` only. Optional modules
  should not add local `replace` directives or sibling requirements just to support
  workspace development outside `go.work`.
- Root releases use tags such as `v0.1.3`. Optional modules are released with
  path-prefixed tags such as `cmd/v0.1.3`, `provider/docker/v0.1.3`, or
  `cluster/raftnode/v0.1.3`.
- Middleware config type is strict: empty type means builtin, non-empty unknown values fail compilation.
- Builtin middleware covers path transforms, redirects, headers, secure headers, CORS, rate limit, circuit breaker, basic auth, forward auth, gzip compression, IP allow list, body limits, and chains.
- `cluster/raftnode` can use an externally owned Dragonboat `NodeHost`; callers own the data directories and must isolate Dragonboat `DeploymentID`, group IDs, node IDs, and NodeHost/WAL directories.
- `cluster/raftnode` exposes a discovery interface. The built-in memberlist implementation only discovers candidate peers; Raft membership remains leader-controlled through Dragonboat config changes.

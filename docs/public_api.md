# Public API Matrix

This repository is library-first. The root package is the preferred import path for
embedded users; subpackages remain available for advanced wiring and optional modules.

## Core Module

| Package | Stability target | Purpose |
| --- | --- | --- |
| `github.com/arcgolabs/vela` | Public | Primary embedded API, functional options, runtime builders, config builders. |
| `github.com/arcgolabs/vela/config` | Public DTO | HCL/JSON config structs, validation, fingerprinting. Native slices/maps remain here for decoder compatibility. |
| `github.com/arcgolabs/vela/runtime` | Public advanced | Compiled data-plane model, matcher, middleware registry, metrics contracts. Uses collectionx containers. |
| `github.com/arcgolabs/vela/provider` | Public advanced | Provider interfaces, config builder implementation, events, fallback provider, closers. |
| `github.com/arcgolabs/vela/provider/merged` | Public advanced | Multi-source merge, validate, compile, debounce, fingerprint dedupe. |
| `github.com/arcgolabs/vela/provider/memoryconfig` | Public advanced | Mutable in-memory config source for embedded dynamic updates. |
| `github.com/arcgolabs/vela/provider/static` | Public advanced | Static compiled snapshot provider. |
| `github.com/arcgolabs/vela/provider/staticconfig` | Public advanced | Static config DTO provider. |
| `github.com/arcgolabs/vela/compiler` | Advanced | Config-to-runtime compiler. Useful for custom provider pipelines. |
| `github.com/arcgolabs/vela/gateway` | Advanced | Lower-level gateway orchestration used by root package. |
| `github.com/arcgolabs/vela/proxy` | Internal-leaning | Built-in oxy proxy construction. Prefer root/runtime APIs unless customizing internals. |

## Optional Modules

| Module | Purpose |
| --- | --- |
| `github.com/arcgolabs/vela/cmd` | Standalone `velad` binary. |
| `github.com/arcgolabs/vela/cluster/raftnode` | Optional HashiCorp Raft control-plane adapter. |
| `github.com/arcgolabs/vela/observability/prometheus` | Optional Prometheus adapter for metrics exposition. |
| `github.com/arcgolabs/vela/provider/docker` | Optional Docker label-driven config source. |
| `github.com/arcgolabs/vela/provider/file` | Optional HCL snapshot provider. |
| `github.com/arcgolabs/vela/provider/fileconfig` | Optional HCL config source provider. |
| `github.com/arcgolabs/vela/provider/k8s` | Optional K8s-like config source. |
| `github.com/arcgolabs/vela/examples/*` | Example consumers, not API. |

## Compatibility Notes

- The root package should remain the stable surface for typical embedded usage.
- `config` DTO structs intentionally use native Go collections because HCL/JSON decoding is their boundary.
- Compiled runtime structs intentionally use `collectionx` containers.
- Admin HTTP responses are stable plain JSON DTOs and should not expose collectionx serialization details.
- Middleware config type is strict: empty type means builtin, non-empty unknown values fail compilation.

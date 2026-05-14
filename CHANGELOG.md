# Changelog

## Unreleased

## v0.1.3 - 2026-05-14

- Added CI golangci-lint coverage for root and workspace modules.
- Documented go.work-only local module development and path-prefixed submodule release tags.
- Added a Docker Compose gossip cluster smoke test that validates metadata, data, and certificates Raft voter convergence.
- Fixed reload restarts so the replacement health checker keeps running after static runtime changes.
- Reduced startup log noise by moving internal lifecycle logs to debug and suppressing first health-down transitions at info level.

## v0.1.2 - 2026-05-14

- Added built-in forward auth middleware and Traefik-compatible `forwardauth.*` labels.
- Added gossip-based cluster discovery backed by memberlist, with Dragonboat membership still controlled by the Raft leader.
- Added container-validated auto-discovery flow for metadata, data, and certificates Raft groups.
- Updated selected dependencies while preserving Dragonboat-compatible dependency boundaries.
- Fixed extension component example module metadata for `logx`.

## v0.1.1 - 2026-05-10

- Expanded Traefik-compatible Docker labels for basic auth, compression, IP allow lists, CORS, and rate limits.
- Added builtin basic auth, gzip compression, and IP allow list middleware.
- Added reload status admin view with route/service/endpoint diff information.
- Added Raft applied-state persistence regression coverage.
- Added GHCR container image publishing for `valed` release tags.
- Added internal Go benchmarks plus a Docker-based Vale/Traefik/Caddy comparison harness.
- Switched Docker comparison benchmarks to use the GitHub-published Vale image by default, with explicit local-build opt-in.
- Added fixed UPX optimization to source-built and release container images.
- Replaced the public runtime plugin API with compile-time library extension registries.
- Added embedded examples for custom middleware, remote JWT validation, custom config providers, and extension component wiring.

## v0.1.0 - 2026-05-08

- Library-first root API under `github.com/arcgolabs/vale`.
- Collectionx-backed compiled runtime API and builder helpers.
- Built-in `oxy` reverse proxy engine.
- Static config, file config, merged providers, reload debounce, and config fingerprint dedupe.
- Built-in middleware plus embedded middleware registry.
- Static TLS and ACME support with secure defaults.
- Observabilityx-backed request, reload, health, and active object metrics.
- go-memdb-backed runtime catalog for route admin queries and future control-plane diffing.
- Typed Raft FSM state for route sync commands with storx/bboltx persistence.
- Optional workspace modules for `cmd`, raft, prometheus, docker, file providers, k8s provider, and examples.

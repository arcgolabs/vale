# Changelog

## Unreleased

- Expanded Traefik-compatible Docker labels for basic auth, compression, IP allow lists, CORS, and rate limits.
- Added builtin basic auth, gzip compression, and IP allow list middleware.
- Added reload status admin view with route/service/endpoint diff information.
- Added Raft applied-state persistence regression coverage.

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

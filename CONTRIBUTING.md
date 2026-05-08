# Contributing

Vale is maintained as a library-first Go workspace. The root module is the
preferred public API for embedding, and `cmd` contains the standalone binary
wiring.

## Local Development

Use the workspace for local module wiring:

```bash
go work sync
```

Do not commit local `replace` directives for modules that already live in this
repository. Each module should keep publishable module paths in its `go.mod`;
`go.work` is responsible for local development and release validation.

## Validation

Run these checks before opening a pull request:

```bash
go test ./...
go test ./cluster/raftnode/... ./cmd/... ./observability/prometheus/... ./provider/docker/... ./provider/file/... ./provider/fileconfig/... ./provider/k8s/... ./examples/embedded_multi_provider/... ./examples/embedded_static_config/...
go vet ./...
go vet ./cluster/raftnode/... ./cmd/... ./observability/prometheus/... ./provider/docker/... ./provider/file/... ./provider/fileconfig/... ./provider/k8s/... ./examples/embedded_multi_provider/... ./examples/embedded_static_config/...
```

The GitHub CI workflow runs the same test and vet set.

## API Guidelines

- Prefer the root `github.com/arcgolabs/vale` package for public embedded APIs.
- Keep `config` decoder DTOs friendly to HCL/JSON and Go standard tooling.
- Prefer `collectionx` collection types for runtime-facing public APIs and
  internal collection-heavy code.
- Keep optional integrations in optional modules unless the dependency is part
  of the core runtime contract.
- Do not expose reverse proxy engine selection as user configuration; Oxy is the
  built-in default engine.

## Commit Scope

Keep changes focused enough to review in one pass. Runtime behavior changes
should include tests or explicit documentation updates. Release infrastructure
changes should avoid changing runtime behavior.

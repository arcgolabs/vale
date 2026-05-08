# Vela v0.1.0 Release Checklist

## Required Checks

- GitHub CI is green on the release commit.
- `go test ./...`
- `go vet ./...`
- Workspace module tests:
  `go test ./cluster/raftnode/... ./cmd/... ./observability/prometheus/... ./provider/docker/... ./provider/file/... ./provider/fileconfig/... ./provider/k8s/... ./examples/embedded_multi_provider/... ./examples/embedded_static_config/...`
- Workspace module vet:
  `go vet ./cluster/raftnode/... ./cmd/... ./observability/prometheus/... ./provider/docker/... ./provider/file/... ./provider/fileconfig/... ./provider/k8s/... ./examples/embedded_multi_provider/... ./examples/embedded_static_config/...`

## API Review

- Root package remains the preferred embedded API.
- New root exports are reflected in README examples.
- `config` remains decoder-friendly with native maps/slices.
- Runtime/admin HTTP views do not expose collectionx JSON behavior.
- Unknown middleware types fail compile rather than falling back silently.

## Module Review

- No local `replace` directives are committed.
- Optional modules keep publishable module paths.
- `go.work` is local workspace wiring only.
- GitHub repository name is `arcgolabs/vela`, or the redirect from the previous
  repository name is verified for `go get github.com/arcgolabs/vela`.

## Release Notes

- Update `CHANGELOG.md`.
- Confirm the MIT license is included.
- Review `CONTRIBUTING.md` and `SECURITY.md` for public repository accuracy.
- Tag the root module.
- Tag optional modules only when their public contracts are ready for consumers.

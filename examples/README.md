# Vale Embedded Examples

These examples are intentionally small library-first entrypoints. They show how
to import Vale and compose gateway behavior without runtime plugins.

| Example | Scenario |
| --- | --- |
| `embedded_static_config` | Static in-process `config.Config` built with `vale.NewConfigBuilder`. |
| `embedded_builder_components` | `GatewayBuilder` composition with a custom middleware component. |
| `embedded_multi_provider` | Multiple optional providers merged into one gateway. |
| `embedded_custom_middleware` | Compile-time extension registering a custom HTTP middleware type. |
| `embedded_jwt_middleware` | Custom gateway middleware that calls an external in-memory auth service to validate JWT Bearer tokens. |
| `embedded_custom_provider` | Compile-time extension registering a custom config provider factory. |
| `embedded_extension_components` | Registry-based certificate storage, observability, and cluster factory wiring. |

Run an example from its directory with `go run .`. The examples use the root
module through `go.work` during local development; they do not use local
`replace` directives.

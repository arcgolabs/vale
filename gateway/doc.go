// Package gateway embeds Vela in other Go programs: build a Gateway with functional
// options, call Start to listen on entrypoints and on the admin address from the
// compiled snapshot, then Stop for graceful shutdown.
//
// Typical wiring combines the built-in default config, merged config providers
// from WithConfigSourceProviders, or a fixed snapshot via WithStaticSnapshot.
// Enable optional control-plane clustering with WithClusterFactory, custom
// runtime middleware with WithMiddlewareRegistry, and metrics with WithMetricsFactory.
// Pass WithEventBus to share lifecycle events across your app or use Events(); if no bus was
// passed, Gateway creates an eventx bus and closes it on Stop only when Gateway owned it internally.
//
// Stable low-level import path: github.com/arcgolabs/vale/gateway
package gateway

// Package gateway embeds Vela in other Go programs: build a Gateway with functional
// options, call Start to listen on entrypoints and on the admin address from the
// compiled snapshot, then Stop for graceful shutdown.
//
// Typical wiring combines file-backed HCL paths (defaults to ./vela.hcl with watch on),
// merged config providers from WithConfigSourceProviders or WithDockerProvider, or a fixed
// snapshot via WithStaticSnapshot. Enable optional Raft control-plane metadata with WithRaftCluster.
// Pass WithEventBus to share lifecycle events across your app or use Events(); if no bus was
// passed, Gateway creates one and closes it on Stop only when Gateway owned it internally.
//
// Stable import path: github.com/arcgolabs/gateway/gateway
package gateway

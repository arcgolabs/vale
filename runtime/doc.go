// Package runtime contains Vela's collectionx-backed compiled data plane model,
// route matcher, middleware chain, health checker, access logging, and metrics
// recorder contracts.
//
// Config file DTOs live in package config. Runtime values are already compiled
// and are suitable for embedded library users that want to construct snapshots
// directly with NewSnapshot, NewService, NewRoute, and related helpers.
package runtime

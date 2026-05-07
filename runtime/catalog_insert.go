package runtime

import (
	"fmt"

	"github.com/arcgolabs/collectionx/mapping"
	"github.com/hashicorp/go-memdb"
	"github.com/samber/oops"
)

func insertCatalogServices(txn *memdb.Txn, snapshot *CompiledSnapshot) error {
	if snapshot.Services == nil {
		return nil
	}
	var insertErr error
	snapshot.Services.Range(func(_ string, service *ServiceRuntime) bool {
		insertErr = insertCatalogService(txn, service)
		return insertErr == nil
	})
	return insertErr
}

func insertCatalogService(txn *memdb.Txn, service *ServiceRuntime) error {
	if service == nil {
		return nil
	}
	if err := txn.Insert(catalogTableService, ServiceRecord{
		Name:     service.Name,
		Strategy: service.Strategy,
	}); err != nil {
		return oops.
			In("runtime").
			With("table", catalogTableService, "service", service.Name).
			Wrapf(err, "insert runtime catalog service")
	}
	return insertCatalogEndpoints(txn, service)
}

func insertCatalogEndpoints(txn *memdb.Txn, service *ServiceRuntime) error {
	if service.Endpoints == nil {
		return nil
	}
	var insertErr error
	service.Endpoints.Range(func(index int, endpoint *EndpointRuntime) bool {
		if endpoint == nil || endpoint.URL == nil {
			return true
		}
		if err := txn.Insert(catalogTableEndpoint, EndpointRecord{
			ID:      fmt.Sprintf("%s/%06d", service.Name, index),
			Service: service.Name,
			URL:     endpoint.URL.String(),
			Weight:  endpoint.Weight,
		}); err != nil {
			insertErr = oops.
				In("runtime").
				With("table", catalogTableEndpoint, "service", service.Name, "endpoint", endpoint.URL.String()).
				Wrapf(err, "insert runtime catalog endpoint")
			return false
		}
		return true
	})
	return insertErr
}

func insertCatalogRoutes(txn *memdb.Txn, snapshot *CompiledSnapshot) error {
	if snapshot.RoutesByEntrypoint == nil {
		return nil
	}
	var insertErr error
	middlewares := mapping.NewMap[string, MiddlewareRecord]()
	snapshot.RoutesByEntrypoint.Range(func(entrypoint string, routes []*CompiledRoute) bool {
		insertErr = insertCatalogRouteGroup(txn, middlewares, entrypoint, routes)
		return insertErr == nil
	})
	if insertErr != nil {
		return insertErr
	}
	return insertCatalogMiddlewares(txn, middlewares)
}

func insertCatalogRouteGroup(txn *memdb.Txn, middlewares *mapping.Map[string, MiddlewareRecord], entrypoint string, routes []*CompiledRoute) error {
	for _, route := range routes {
		if route == nil {
			continue
		}
		if err := insertCatalogRoute(txn, entrypoint, route); err != nil {
			return err
		}
		collectCatalogMiddlewares(middlewares, route)
	}
	return nil
}

func insertCatalogRoute(txn *memdb.Txn, entrypoint string, route *CompiledRoute) error {
	if err := txn.Insert(catalogTableRoute, RouteRecord{
		Name:       route.Name,
		Entrypoint: entrypoint,
		Host:       route.Host,
		PathPrefix: route.PathPrefix,
		Method:     route.Method,
		Service:    routeServiceName(route),
	}); err != nil {
		return oops.
			In("runtime").
			With("table", catalogTableRoute, "route", route.Name, "entrypoint", entrypoint).
			Wrapf(err, "insert runtime catalog route")
	}
	return nil
}

func routeServiceName(route *CompiledRoute) string {
	if route.Service == nil {
		return ""
	}
	return route.Service.Name
}

func collectCatalogMiddlewares(middlewares *mapping.Map[string, MiddlewareRecord], route *CompiledRoute) {
	if route.Middlewares == nil {
		return
	}
	route.Middlewares.Range(func(_ int, middleware MiddlewareRuntime) bool {
		middlewares.Set(middleware.Name, MiddlewareRecord{
			Name: middleware.Name,
			Type: middleware.Type,
		})
		return true
	})
}

func insertCatalogMiddlewares(txn *memdb.Txn, middlewares *mapping.Map[string, MiddlewareRecord]) error {
	var middlewareErr error
	middlewares.Range(func(_ string, middleware MiddlewareRecord) bool {
		if err := txn.Insert(catalogTableMiddleware, middleware); err != nil {
			middlewareErr = oops.
				In("runtime").
				With("table", catalogTableMiddleware, "middleware", middleware.Name, "type", middleware.Type).
				Wrapf(err, "insert runtime catalog middleware")
			return false
		}
		return true
	})
	return middlewareErr
}

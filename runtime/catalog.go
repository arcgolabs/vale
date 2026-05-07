package runtime

import (
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/hashicorp/go-memdb"
)

const (
	catalogTableRoute      = "route"
	catalogTableService    = "service"
	catalogTableEndpoint   = "endpoint"
	catalogTableMiddleware = "middleware"
)

// Catalog is a control-plane index over a compiled snapshot. It is not used by
// the request hot path; route matching stays on EntrypointMatcher.
type Catalog struct {
	db *memdb.MemDB
}

type RouteFilter struct {
	Entrypoint string
	Service    string
	Host       string
	PathPrefix string
}

type RouteRecord struct {
	Name       string
	Entrypoint string
	Host       string
	PathPrefix string
	Method     string
	Service    string
}

type ServiceRecord struct {
	Name     string
	Strategy string
}

type EndpointRecord struct {
	ID      string
	Service string
	URL     string
	Weight  int
}

type MiddlewareRecord struct {
	Name string
	Type string
}

func BuildCatalog(snapshot *CompiledSnapshot) (*Catalog, error) {
	db, err := memdb.NewMemDB(catalogSchema())
	if err != nil {
		return nil, err
	}
	txn := db.Txn(true)
	committed := false
	defer func() {
		if !committed {
			txn.Abort()
		}
	}()

	if snapshot != nil {
		if err := insertCatalogServices(txn, snapshot); err != nil {
			return nil, err
		}
		if err := insertCatalogRoutes(txn, snapshot); err != nil {
			return nil, err
		}
	}
	txn.Commit()
	committed = true
	return &Catalog{db: db}, nil
}

func (s *CompiledSnapshot) BuildCatalog() *CompiledSnapshot {
	if s == nil {
		return nil
	}
	catalog, err := BuildCatalog(s)
	if err == nil {
		s.Catalog = catalog
	}
	return s
}

func (s *CompiledSnapshot) QueryRoutes(filter RouteFilter) *collectionlist.List[RouteView] {
	if s == nil {
		return collectionlist.NewList[RouteView]()
	}
	if s.Catalog != nil {
		views, err := s.Catalog.RouteViews(filter)
		if err == nil {
			return views
		}
	}
	return filterRouteViews(s.routesFallback(), filter)
}

func (c *Catalog) RouteViews(filter RouteFilter) (*collectionlist.List[RouteView], error) {
	records, err := c.Routes(filter)
	if err != nil {
		return nil, err
	}
	views := collectionlist.NewListWithCapacity[RouteView](records.Len())
	records.Range(func(_ int, route RouteRecord) bool {
		views.Add(RouteView{
			Name:       route.Name,
			Entrypoint: route.Entrypoint,
			Host:       route.Host,
			PathPrefix: route.PathPrefix,
			Method:     route.Method,
			Service:    route.Service,
		})
		return true
	})
	return views, nil
}

func (c *Catalog) Routes(filter RouteFilter) (*collectionlist.List[RouteRecord], error) {
	routes := collectionlist.NewList[RouteRecord]()
	if c == nil || c.db == nil {
		return routes, nil
	}
	filter = normalizeRouteFilter(filter)
	index, args := routeLookup(filter)
	txn := c.db.Txn(false)
	defer txn.Abort()
	it, err := txn.Get(catalogTableRoute, index, args...)
	if err != nil {
		return nil, err
	}
	for item := it.Next(); item != nil; item = it.Next() {
		route, ok := item.(RouteRecord)
		if !ok {
			return nil, fmt.Errorf("catalog route record has unexpected type %T", item)
		}
		if routeMatchesFilter(route, filter) {
			routes.Add(route)
		}
	}
	routes.Sort(func(left RouteRecord, right RouteRecord) int {
		return strings.Compare(left.Name, right.Name)
	})
	return routes, nil
}

func (s *CompiledSnapshot) routesFallback() *collectionlist.List[RouteView] {
	routeList := collectionlist.NewList[RouteView]()
	if s == nil || s.RoutesByEntrypoint == nil {
		return routeList
	}
	s.RoutesByEntrypoint.Range(func(entrypoint string, routes []*CompiledRoute) bool {
		for _, route := range routes {
			serviceName := ""
			if route.Service != nil {
				serviceName = route.Service.Name
			}
			routeList.Add(RouteView{
				Name:       route.Name,
				Entrypoint: entrypoint,
				Host:       route.Host,
				PathPrefix: route.PathPrefix,
				Method:     route.Method,
				Service:    serviceName,
			})
		}
		return true
	})
	return routeList
}

func filterRouteViews(routes *collectionlist.List[RouteView], filter RouteFilter) *collectionlist.List[RouteView] {
	filter = normalizeRouteFilter(filter)
	filtered := collectionlist.NewList[RouteView]()
	if routes == nil {
		return filtered
	}
	routes.Range(func(_ int, route RouteView) bool {
		if routeMatchesFilter(RouteRecord(route), filter) {
			filtered.Add(route)
		}
		return true
	})
	return filtered
}

func insertCatalogServices(txn *memdb.Txn, snapshot *CompiledSnapshot) error {
	if snapshot.Services == nil {
		return nil
	}
	var insertErr error
	snapshot.Services.Range(func(_ string, service *ServiceRuntime) bool {
		if service == nil {
			return true
		}
		if err := txn.Insert(catalogTableService, ServiceRecord{
			Name:     service.Name,
			Strategy: service.Strategy,
		}); err != nil {
			insertErr = err
			return false
		}
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
				insertErr = err
				return false
			}
			return true
		})
		return insertErr == nil
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
		for _, route := range routes {
			if route == nil {
				continue
			}
			serviceName := ""
			if route.Service != nil {
				serviceName = route.Service.Name
			}
			if err := txn.Insert(catalogTableRoute, RouteRecord{
				Name:       route.Name,
				Entrypoint: entrypoint,
				Host:       route.Host,
				PathPrefix: route.PathPrefix,
				Method:     route.Method,
				Service:    serviceName,
			}); err != nil {
				insertErr = err
				return false
			}
			if route.Middlewares != nil {
				route.Middlewares.Range(func(_ int, middleware MiddlewareRuntime) bool {
					middlewares.Set(middleware.Name, MiddlewareRecord{
						Name: middleware.Name,
						Type: middleware.Type,
					})
					return true
				})
			}
		}
		return true
	})
	if insertErr != nil {
		return insertErr
	}
	var middlewareErr error
	middlewares.Range(func(_ string, middleware MiddlewareRecord) bool {
		if err := txn.Insert(catalogTableMiddleware, middleware); err != nil {
			middlewareErr = err
			return false
		}
		return true
	})
	return middlewareErr
}

func routeLookup(filter RouteFilter) (string, []any) {
	switch {
	case filter.Entrypoint != "":
		return "entrypoint", []any{filter.Entrypoint}
	case filter.Service != "":
		return "service", []any{filter.Service}
	case filter.Host != "":
		return "host", []any{filter.Host}
	case filter.PathPrefix != "":
		return "path_prefix", []any{filter.PathPrefix}
	default:
		return "id", nil
	}
}

func routeMatchesFilter(route RouteRecord, filter RouteFilter) bool {
	if filter.Entrypoint != "" && route.Entrypoint != filter.Entrypoint {
		return false
	}
	if filter.Service != "" && route.Service != filter.Service {
		return false
	}
	if filter.Host != "" && route.Host != filter.Host {
		return false
	}
	if filter.PathPrefix != "" && route.PathPrefix != filter.PathPrefix {
		return false
	}
	return true
}

func normalizeRouteFilter(filter RouteFilter) RouteFilter {
	filter.Entrypoint = strings.TrimSpace(filter.Entrypoint)
	filter.Service = strings.TrimSpace(filter.Service)
	filter.Host = strings.ToLower(strings.TrimSpace(filter.Host))
	filter.PathPrefix = strings.TrimSpace(filter.PathPrefix)
	return filter
}

func catalogSchema() *memdb.DBSchema {
	return &memdb.DBSchema{
		Tables: map[string]*memdb.TableSchema{
			catalogTableRoute: {
				Name: catalogTableRoute,
				Indexes: map[string]*memdb.IndexSchema{
					"id":          stringIndex("id", "Name", true),
					"entrypoint":  stringIndex("entrypoint", "Entrypoint", false),
					"service":     stringIndex("service", "Service", false),
					"host":        stringIndex("host", "Host", false),
					"path_prefix": stringIndex("path_prefix", "PathPrefix", false),
				},
			},
			catalogTableService: {
				Name: catalogTableService,
				Indexes: map[string]*memdb.IndexSchema{
					"id": stringIndex("id", "Name", true),
				},
			},
			catalogTableEndpoint: {
				Name: catalogTableEndpoint,
				Indexes: map[string]*memdb.IndexSchema{
					"id":      stringIndex("id", "ID", true),
					"service": stringIndex("service", "Service", false),
				},
			},
			catalogTableMiddleware: {
				Name: catalogTableMiddleware,
				Indexes: map[string]*memdb.IndexSchema{
					"id":   stringIndex("id", "Name", true),
					"type": stringIndex("type", "Type", false),
				},
			},
		},
	}
}

func stringIndex(name string, field string, unique bool) *memdb.IndexSchema {
	return &memdb.IndexSchema{
		Name:    name,
		Unique:  unique,
		Indexer: &memdb.StringFieldIndex{Field: field},
	}
}

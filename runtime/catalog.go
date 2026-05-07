package runtime

import (
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/hashicorp/go-memdb"
	"github.com/samber/oops"
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
		return nil, oops.
			In("runtime").
			Wrapf(err, "create runtime catalog database")
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
			return nil, oops.
				In("runtime").
				Wrapf(err, "insert runtime catalog services")
		}
		if err := insertCatalogRoutes(txn, snapshot); err != nil {
			return nil, oops.
				In("runtime").
				Wrapf(err, "insert runtime catalog routes")
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
		return nil, oops.
			In("runtime").
			With("entrypoint", filter.Entrypoint, "service", filter.Service, "host", filter.Host, "path_prefix", filter.PathPrefix).
			Wrapf(err, "query runtime route views")
	}
	views := collectionlist.NewListWithCapacity[RouteView](records.Len())
	records.Range(func(_ int, route RouteRecord) bool {
		views.Add(RouteView(route))
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
		return nil, oops.
			In("runtime").
			With("table", catalogTableRoute, "index", index).
			Wrapf(err, "query runtime route catalog")
	}
	for item := it.Next(); item != nil; item = it.Next() {
		route, ok := item.(RouteRecord)
		if !ok {
			return nil, oops.
				In("runtime").
				With("table", catalogTableRoute, "index", index, "type", fmt.Sprintf("%T", item)).
				New("catalog route record has unexpected type")
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

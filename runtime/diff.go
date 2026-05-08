package runtime

import (
	"fmt"
	"strconv"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type ResourceDiff struct {
	Added   *collectionlist.List[string] `json:"added"`
	Removed *collectionlist.List[string] `json:"removed"`
	Changed *collectionlist.List[string] `json:"changed"`
}

type SnapshotDiff struct {
	Routes    ResourceDiff `json:"routes"`
	Services  ResourceDiff `json:"services"`
	Endpoints ResourceDiff `json:"endpoints"`
}

func DiffSnapshots(current, next *CompiledSnapshot) SnapshotDiff {
	return SnapshotDiff{
		Routes:    diffNamedRecords(routeFingerprints(current), routeFingerprints(next)),
		Services:  diffNamedRecords(serviceFingerprints(current), serviceFingerprints(next)),
		Endpoints: diffNamedRecords(endpointFingerprints(current), endpointFingerprints(next)),
	}
}

func (d SnapshotDiff) HasChanges() bool {
	return d.Routes.HasChanges() || d.Services.HasChanges() || d.Endpoints.HasChanges()
}

func (d ResourceDiff) HasChanges() bool {
	return listHasValues(d.Added) || listHasValues(d.Removed) || listHasValues(d.Changed)
}

func EmptyResourceDiff() ResourceDiff {
	return ResourceDiff{
		Added:   collectionlist.NewList[string](),
		Removed: collectionlist.NewList[string](),
		Changed: collectionlist.NewList[string](),
	}
}

func diffNamedRecords(current, next *mapping.Map[string, string]) ResourceDiff {
	diff := EmptyResourceDiff()
	next.Range(func(name string, fingerprint string) bool {
		currentFingerprint, exists := current.Get(name)
		switch {
		case !exists:
			diff.Added.Add(name)
		case currentFingerprint != fingerprint:
			diff.Changed.Add(name)
		}
		return true
	})
	current.Range(func(name string, _ string) bool {
		if _, exists := next.Get(name); !exists {
			diff.Removed.Add(name)
		}
		return true
	})
	sortStringList(diff.Added)
	sortStringList(diff.Removed)
	sortStringList(diff.Changed)
	return diff
}

func routeFingerprints(snapshot *CompiledSnapshot) *mapping.Map[string, string] {
	records := mapping.NewMap[string, string]()
	if snapshot == nil || snapshot.RoutesByEntrypoint == nil {
		return records
	}
	snapshot.RoutesByEntrypoint.Range(func(entrypoint string, routes []*CompiledRoute) bool {
		collectionlist.NewList(routes...).Range(func(_ int, route *CompiledRoute) bool {
			if route == nil {
				return true
			}
			records.Set(route.Name, strings.Join([]string{
				entrypoint,
				route.Host,
				route.PathPrefix,
				route.Method,
				routeServiceName(route),
				middlewareFingerprint(route.Middlewares),
			}, "\x00"))
			return true
		})
		return true
	})
	return records
}

func serviceFingerprints(snapshot *CompiledSnapshot) *mapping.Map[string, string] {
	records := mapping.NewMap[string, string]()
	if snapshot == nil || snapshot.Services == nil {
		return records
	}
	snapshot.Services.Range(func(name string, service *ServiceRuntime) bool {
		if service == nil {
			return true
		}
		records.Set(name, service.Strategy+"\x00"+serviceEndpointFingerprint(service))
		return true
	})
	return records
}

func endpointFingerprints(snapshot *CompiledSnapshot) *mapping.Map[string, string] {
	records := mapping.NewMap[string, string]()
	if snapshot == nil || snapshot.Services == nil {
		return records
	}
	snapshot.Services.Range(func(serviceName string, service *ServiceRuntime) bool {
		if service == nil || service.Endpoints == nil {
			return true
		}
		service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
			if endpoint == nil || endpoint.URL == nil {
				return true
			}
			name := fmt.Sprintf("%s/%s", serviceName, endpoint.URL.String())
			records.Set(name, strconv.Itoa(endpoint.Weight))
			return true
		})
		return true
	})
	return records
}

func middlewareFingerprint(middlewares *collectionlist.List[MiddlewareRuntime]) string {
	if middlewares == nil || middlewares.IsEmpty() {
		return ""
	}
	names := collectionlist.MapList(middlewares, func(_ int, middleware MiddlewareRuntime) string {
		return middleware.Name
	})
	sortStringList(names)
	return names.Join(",")
}

func serviceEndpointFingerprint(service *ServiceRuntime) string {
	if service == nil || service.Endpoints == nil {
		return ""
	}
	endpoints := collectionlist.NewListWithCapacity[string](service.Endpoints.Len())
	service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
		if endpoint == nil || endpoint.URL == nil {
			return true
		}
		endpoints.Add(fmt.Sprintf("%s#%d", endpoint.URL.String(), endpoint.Weight))
		return true
	})
	sortStringList(endpoints)
	return endpoints.Join(",")
}

func listHasValues(values *collectionlist.List[string]) bool {
	return values != nil && !values.IsEmpty()
}

func sortStringList(values *collectionlist.List[string]) {
	if values == nil {
		return
	}
	values.Sort(strings.Compare)
}

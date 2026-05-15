package runtime

import collectionlist "github.com/arcgolabs/collectionx/list"

func (s *CompiledSnapshot) routesFallback(filter RouteFilter) *collectionlist.List[RouteView] {
	routeList := collectionlist.NewList[RouteView]()
	if s == nil || s.RoutesByEntrypoint == nil {
		return routeList
	}
	filter = normalizeRouteFilter(filter)
	s.rangeFallbackRoutes(filter, func(route *CompiledRoute) {
		addRouteViewIfMatched(routeList, route, filter)
	})
	return routeList
}

func (s *CompiledSnapshot) rangeFallbackRoutes(filter RouteFilter, yield func(*CompiledRoute)) {
	if filter.Entrypoint != "" {
		for _, route := range s.RoutesByEntrypoint.Get(filter.Entrypoint) {
			yield(route)
		}
		return
	}
	s.RoutesByEntrypoint.Range(func(_ string, routes []*CompiledRoute) bool {
		for _, route := range routes {
			yield(route)
		}
		return true
	})
}

func addRouteViewIfMatched(routeList *collectionlist.List[RouteView], route *CompiledRoute, filter RouteFilter) {
	view := buildRouteView(route)
	if view == nil || !routeViewMatchesFilter(*view, filter) {
		return
	}
	routeList.Add(*view)
}

package runtime

func hasPredicate(route *CompiledRoute, predicate int) bool {
	if route == nil {
		return false
	}
	if route.Predicates != nil {
		return route.Predicates.Contains(predicate)
	}
	switch predicate {
	case PredicateHost:
		return route.Host != ""
	case PredicatePathPrefix:
		return route.PathPrefix != ""
	case PredicateMethod:
		return route.Method != ""
	case PredicateHeaders:
		return route.Headers != nil && route.Headers.Len() > 0
	default:
		return false
	}
}

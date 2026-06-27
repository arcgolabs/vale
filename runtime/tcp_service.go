package runtime

import (
	"sort"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/samber/oops"
)

func (s *CompiledSnapshot) AddTCPService(service *TCPServiceRuntime) *CompiledSnapshot {
	if s == nil || service == nil {
		return s
	}
	if s.TCPServices == nil {
		s.TCPServices = mapping.NewMap[string, *TCPServiceRuntime]()
	}
	s.TCPServices.Set(service.Name, service)
	return s
}

func (s *CompiledSnapshot) AddTCPRoute(route *CompiledTCPRoute) *CompiledSnapshot {
	if s == nil || route == nil {
		return s
	}
	if s.TCPRoutes == nil {
		s.TCPRoutes = mapping.NewMap[string, *CompiledTCPRoute]()
	}
	s.TCPRoutes.Set(route.Entrypoint, route)
	return s
}

func NewTCPService(name, strategy string, endpoints ...*TCPEndpointRuntime) *TCPServiceRuntime {
	if strategy == "" {
		strategy = "round_robin"
	}
	service := &TCPServiceRuntime{
		Name:      strings.TrimSpace(name),
		Strategy:  strings.TrimSpace(strategy),
		Endpoints: collectionlist.NewListWithCapacity[*TCPEndpointRuntime](len(endpoints), endpoints...),
	}
	service.BuildSlots()
	return service
}

func NewTCPEndpoint(address string, weight int) *TCPEndpointRuntime {
	if weight <= 0 {
		weight = 1
	}
	endpoint := &TCPEndpointRuntime{
		Address: strings.TrimSpace(address),
		Weight:  weight,
	}
	endpoint.Healthy.Store(true)
	return endpoint
}

func NewTCPRoute(name, entrypoint string, service *TCPServiceRuntime) *CompiledTCPRoute {
	return &CompiledTCPRoute{
		Name:       strings.TrimSpace(name),
		Entrypoint: strings.TrimSpace(entrypoint),
		Service:    service,
	}
}

func (s *TCPServiceRuntime) BuildSlots() {
	s.weightedRanges = collectionlist.NewList[weightedEndpointRange]()
	s.totalWeight = 0
	if s.Strategy != "weighted_round_robin" || s.Endpoints == nil {
		return
	}
	s.Endpoints.Range(func(idx int, endpoint *TCPEndpointRuntime) bool {
		weight := endpoint.Weight
		if weight <= 0 {
			weight = 1
		}
		s.totalWeight += uint64(weight)
		s.weightedRanges.Add(weightedEndpointRange{index: idx, maxExclusive: s.totalWeight})
		return true
	})
}

func (s *TCPServiceRuntime) Pick() (*TCPEndpointRuntime, error) {
	endpointCount := s.Endpoints.Len()
	if endpointCount == 0 {
		return nil, oops.
			In("runtime").
			With("service", s.Name).
			New("tcp service has no endpoints")
	}
	if endpointCount == 1 {
		if endpoint := s.pickOnlyEndpoint(); endpoint != nil {
			return endpoint, nil
		}
		return nil, noHealthyTCPEndpointError(s, endpointCount)
	}
	if s.Strategy == "weighted_round_robin" && !s.weightedRanges.IsEmpty() && s.totalWeight > 0 {
		if endpoint := s.pickWeightedEndpoint(); endpoint != nil {
			return endpoint, nil
		}
	} else if endpoint := s.pickRoundRobinEndpoint(endpointCount); endpoint != nil {
		return endpoint, nil
	}
	return nil, noHealthyTCPEndpointError(s, endpointCount)
}

func noHealthyTCPEndpointError(s *TCPServiceRuntime, endpointCount int) error {
	return oops.
		In("runtime").
		With("service", s.Name, "endpoints", endpointCount, "strategy", s.Strategy).
		New("no healthy tcp endpoint")
}

func (s *TCPServiceRuntime) pickOnlyEndpoint() *TCPEndpointRuntime {
	endpoint, _ := s.Endpoints.GetFirst()
	if endpoint.Healthy.Load() {
		return endpoint
	}
	return nil
}

func (s *TCPServiceRuntime) pickWeightedEndpoint() *TCPEndpointRuntime {
	rangeCount := s.weightedRanges.Len()
	start := s.weightedRangeIndex(s.nextTicket(s.totalWeight))
	for offset := range rangeCount {
		weightedRange, _ := s.weightedRanges.Get((start + offset) % rangeCount)
		endpoint, _ := s.Endpoints.Get(weightedRange.index)
		if endpoint.Healthy.Load() {
			return endpoint
		}
	}
	return nil
}

func (s *TCPServiceRuntime) weightedRangeIndex(ticket uint64) int {
	return findWeightedRangeIndex(s.weightedRanges, ticket)
}

func findWeightedRangeIndex(ranges *collectionlist.List[weightedEndpointRange], ticket uint64) int {
	if ranges == nil {
		return 0
	}
	return sort.Search(ranges.Len(), func(index int) bool {
		weightedRange, _ := ranges.Get(index)
		return ticket < weightedRange.maxExclusive
	})
}

func (s *TCPServiceRuntime) pickRoundRobinEndpoint(endpointCount int) *TCPEndpointRuntime {
	start := s.nextStart(endpointCount)
	for offset := range endpointCount {
		endpoint, _ := s.Endpoints.Get((start + offset) % endpointCount)
		if endpoint.Healthy.Load() {
			return endpoint
		}
	}
	return nil
}

func (s *TCPServiceRuntime) nextStart(count int) int {
	if count <= 0 {
		return 0
	}
	slot := s.rrCounter.Add(1) % uint64(count)
	return safeUint64ToInt(slot)
}

func (s *TCPServiceRuntime) nextTicket(totalWeight uint64) uint64 {
	if totalWeight == 0 {
		return 0
	}
	return s.rrCounter.Add(1) % totalWeight
}

package runtime

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

type weightedEndpointRange struct {
	index        int
	maxExclusive uint64
}

func (s *ServiceRuntime) BuildSlots() {
	s.weightedRanges = collectionlist.NewList[weightedEndpointRange]()
	s.totalWeight = 0
	if s.Strategy != "weighted_round_robin" {
		return
	}
	if s.Endpoints == nil {
		return
	}
	s.Endpoints.Range(func(idx int, endpoint *EndpointRuntime) bool {
		weight := endpoint.Weight
		if weight <= 0 {
			weight = 1
		}
		s.totalWeight += uint64(weight)
		s.weightedRanges.Add(weightedEndpointRange{index: idx, maxExclusive: s.totalWeight})
		return true
	})
}

func (s *ServiceRuntime) Pick() (*EndpointRuntime, error) {
	endpointCount := s.Endpoints.Len()
	if endpointCount == 0 {
		return nil, oops.
			In("runtime").
			With("service", s.Name).
			New("service has no endpoints")
	}
	if endpointCount == 1 {
		if endpoint := s.pickOnlyEndpoint(); endpoint != nil {
			return endpoint, nil
		}
		return nil, noHealthyEndpointError(s, endpointCount)
	}
	if s.Strategy == "weighted_round_robin" && !s.weightedRanges.IsEmpty() && s.totalWeight > 0 {
		if endpoint := s.pickWeightedEndpoint(); endpoint != nil {
			return endpoint, nil
		}
	} else if endpoint := s.pickRoundRobinEndpoint(endpointCount); endpoint != nil {
		return endpoint, nil
	}
	return nil, noHealthyEndpointError(s, endpointCount)
}

func noHealthyEndpointError(s *ServiceRuntime, endpointCount int) error {
	return oops.
		In("runtime").
		With("service", s.Name, "endpoints", endpointCount, "strategy", s.Strategy).
		New("no healthy endpoint")
}

func (s *ServiceRuntime) pickOnlyEndpoint() *EndpointRuntime {
	endpoint, _ := s.Endpoints.GetFirst()
	if endpoint.Healthy.Load() {
		return endpoint
	}
	return nil
}

func (s *ServiceRuntime) pickWeightedEndpoint() *EndpointRuntime {
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

func (s *ServiceRuntime) weightedRangeIndex(ticket uint64) int {
	idx := 0
	s.weightedRanges.Range(func(index int, weightedRange weightedEndpointRange) bool {
		if ticket >= weightedRange.maxExclusive {
			return true
		}
		idx = index
		return false
	})
	return idx
}

func (s *ServiceRuntime) pickRoundRobinEndpoint(endpointCount int) *EndpointRuntime {
	start := s.nextStart(endpointCount)
	for offset := range endpointCount {
		endpoint, _ := s.Endpoints.Get((start + offset) % endpointCount)
		if endpoint.Healthy.Load() {
			return endpoint
		}
	}
	return nil
}

func (s *ServiceRuntime) nextStart(count int) int {
	if count <= 0 {
		return 0
	}
	modulo := s.rrCounter.Add(1) % uint64(count)
	for idx := range count {
		if uint64(idx) == modulo {
			return idx
		}
	}
	return 0
}

func (s *ServiceRuntime) nextTicket(totalWeight uint64) uint64 {
	if totalWeight == 0 {
		return 0
	}
	return s.rrCounter.Add(1) % totalWeight
}

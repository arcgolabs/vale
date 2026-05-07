package runtime

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

func (s *ServiceRuntime) BuildSlots() {
	s.weightedSlots = collectionlist.NewList[int]()
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
		for range weight {
			s.weightedSlots.Add(idx)
		}
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
	if s.Strategy == "weighted_round_robin" && !s.weightedSlots.IsEmpty() {
		if endpoint := s.pickWeightedEndpoint(); endpoint != nil {
			return endpoint, nil
		}
	} else if endpoint := s.pickRoundRobinEndpoint(endpointCount); endpoint != nil {
		return endpoint, nil
	}
	return nil, oops.
		In("runtime").
		With("service", s.Name, "endpoints", endpointCount, "strategy", s.Strategy).
		New("no healthy endpoint")
}

func (s *ServiceRuntime) pickWeightedEndpoint() *EndpointRuntime {
	slotCount := s.weightedSlots.Len()
	start := s.nextStart(slotCount)
	for offset := range slotCount {
		slot, _ := s.weightedSlots.Get((start + offset) % slotCount)
		endpoint, _ := s.Endpoints.Get(slot)
		if endpoint.Healthy.Load() {
			return endpoint
		}
	}
	return nil
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

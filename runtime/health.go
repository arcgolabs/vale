package runtime

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

type HealthChecker struct {
	interval time.Duration
	client   *http.Client
	stop     chan struct{}
	logger   *slog.Logger
}

func NewHealthChecker(interval time.Duration, timeout time.Duration) *HealthChecker {
	return NewHealthCheckerWithLogger(interval, timeout, nil)
}

func NewHealthCheckerWithLogger(interval time.Duration, timeout time.Duration, logger *slog.Logger) *HealthChecker {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &HealthChecker{
		interval: interval,
		client: &http.Client{
			Timeout: timeout,
		},
		stop:   make(chan struct{}),
		logger: logger,
	}
}

func (h *HealthChecker) Start(gateway *Gateway) {
	ticker := time.NewTicker(h.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.check(gateway)
			case <-h.stop:
				return
			}
		}
	}()
}

func (h *HealthChecker) Stop() {
	close(h.stop)
}

func (h *HealthChecker) check(gateway *Gateway) {
	if gateway == nil {
		return
	}
	snapshot := gateway.Snapshot()
	if snapshot == nil {
		return
	}
	snapshot.Services.Range(func(_ string, service *ServiceRuntime) bool {
		service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
			ctx, cancel := context.WithTimeout(context.Background(), h.client.Timeout)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL.String(), nil)
			if err != nil {
				h.setEndpointHealth(endpoint, false, "request_build_failed", err)
				gateway.ObserveHealth(endpoint, false)
				cancel()
				return true
			}
			resp, err := h.client.Do(req)
			if err != nil {
				h.setEndpointHealth(endpoint, false, "request_failed", err)
				gateway.ObserveHealth(endpoint, false)
				cancel()
				return true
			}
			_ = resp.Body.Close()
			healthy := resp.StatusCode < http.StatusInternalServerError
			h.setEndpointHealth(endpoint, healthy, "status_checked", nil)
			gateway.ObserveHealth(endpoint, healthy)
			endpoint.LastChecked.Store(time.Now().Unix())
			cancel()
			return true
		})
		return true
	})
}

func (h *HealthChecker) setEndpointHealth(endpoint *EndpointRuntime, healthy bool, reason string, err error) {
	previous := endpoint.Healthy.Swap(healthy)
	if h.logger == nil || previous == healthy {
		return
	}
	args := collectionlist.NewList[any](
		"endpoint", endpoint.URL.String(),
		"healthy", healthy,
		"reason", reason,
	)
	if err != nil {
		args.Add("error", err)
	}
	h.logger.Info("endpoint health changed", args.Values()...)
}

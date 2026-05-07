package runtime

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/samber/oops"
)

type HealthChecker struct {
	interval time.Duration
	client   *http.Client
	stop     chan struct{}
	logger   *slog.Logger
}

func NewHealthChecker(interval, timeout time.Duration) *HealthChecker {
	return NewHealthCheckerWithLogger(interval, timeout, nil)
}

func NewHealthCheckerWithLogger(interval, timeout time.Duration, logger *slog.Logger) *HealthChecker {
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

func (h *HealthChecker) Start(ctx context.Context, gateway *Gateway) {
	ticker := time.NewTicker(h.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				h.check(ctx, gateway)
			case <-ctx.Done():
				return
			case <-h.stop:
				return
			}
		}
	}()
}

func (h *HealthChecker) Stop() {
	close(h.stop)
}

func (h *HealthChecker) check(ctx context.Context, gateway *Gateway) {
	if gateway == nil {
		return
	}
	snapshot := gateway.Snapshot()
	if snapshot == nil {
		return
	}
	snapshot.Services.Range(func(_ string, service *ServiceRuntime) bool {
		h.checkService(ctx, gateway, service)
		return true
	})
}

func (h *HealthChecker) checkService(ctx context.Context, gateway *Gateway, service *ServiceRuntime) {
	service.Endpoints.Range(func(_ int, endpoint *EndpointRuntime) bool {
		h.checkEndpoint(ctx, gateway, endpoint)
		return true
	})
}

func (h *HealthChecker) checkEndpoint(ctx context.Context, gateway *Gateway, endpoint *EndpointRuntime) {
	requestCtx, cancel := context.WithTimeout(ctx, h.client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.URL.String(), http.NoBody)
	if err != nil {
		h.markEndpointUnhealthy(gateway, endpoint, "request_build_failed", oops.
			In("runtime").
			With("url", endpoint.URL.String()).
			Wrapf(err, "build health check request"))
		return
	}
	resp, err := h.client.Do(req)
	if err != nil {
		h.markEndpointUnhealthy(gateway, endpoint, "request_failed", oops.
			In("runtime").
			With("url", endpoint.URL.String()).
			Wrapf(err, "execute health check request"))
		return
	}
	if err := resp.Body.Close(); err != nil && h.logger != nil {
		h.logger.Error("health response body close failed", "url", endpoint.URL.String(), "error", oops.
			In("runtime").
			With("url", endpoint.URL.String()).
			Wrapf(err, "close health check response body"))
	}
	healthy := resp.StatusCode < http.StatusInternalServerError
	h.setEndpointHealth(endpoint, healthy, "status_checked", nil)
	gateway.ObserveHealth(endpoint, healthy)
	endpoint.LastChecked.Store(time.Now().Unix())
}

func (h *HealthChecker) markEndpointUnhealthy(gateway *Gateway, endpoint *EndpointRuntime, reason string, err error) {
	h.setEndpointHealth(endpoint, false, reason, err)
	gateway.ObserveHealth(endpoint, false)
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

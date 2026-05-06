package runtime

import (
	"context"
	"log/slog"
	"net/http"
	"time"
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
				h.check(gateway.Snapshot())
			case <-h.stop:
				return
			}
		}
	}()
}

func (h *HealthChecker) Stop() {
	close(h.stop)
}

func (h *HealthChecker) check(snapshot *CompiledSnapshot) {
	if snapshot == nil {
		return
	}
	for _, service := range snapshot.Services {
		for _, endpoint := range service.Endpoints {
			ctx, cancel := context.WithTimeout(context.Background(), h.client.Timeout)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.URL.String(), nil)
			if err != nil {
				h.setEndpointHealth(endpoint, false, "request_build_failed", err)
				cancel()
				continue
			}
			resp, err := h.client.Do(req)
			if err != nil {
				h.setEndpointHealth(endpoint, false, "request_failed", err)
				cancel()
				continue
			}
			_ = resp.Body.Close()
			h.setEndpointHealth(endpoint, resp.StatusCode < http.StatusInternalServerError, "status_checked", nil)
			endpoint.LastChecked.Store(time.Now().Unix())
			cancel()
		}
	}
}

func (h *HealthChecker) setEndpointHealth(endpoint *EndpointRuntime, healthy bool, reason string, err error) {
	previous := endpoint.Healthy.Swap(healthy)
	if h.logger == nil || previous == healthy {
		return
	}
	args := []any{
		"endpoint", endpoint.URL.String(),
		"healthy", healthy,
		"reason", reason,
	}
	if err != nil {
		args = append(args, "error", err)
	}
	h.logger.Info("endpoint health changed", args...)
}

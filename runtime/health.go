package runtime

import (
	"context"
	"net/http"
	"time"
)

type HealthChecker struct {
	interval time.Duration
	client   *http.Client
	stop     chan struct{}
}

func NewHealthChecker(interval time.Duration, timeout time.Duration) *HealthChecker {
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
		stop: make(chan struct{}),
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
				endpoint.Healthy.Store(false)
				cancel()
				continue
			}
			resp, err := h.client.Do(req)
			if err != nil {
				endpoint.Healthy.Store(false)
				cancel()
				continue
			}
			_ = resp.Body.Close()
			endpoint.Healthy.Store(resp.StatusCode < http.StatusInternalServerError)
			endpoint.LastChecked.Store(time.Now().Unix())
			cancel()
		}
	}
}

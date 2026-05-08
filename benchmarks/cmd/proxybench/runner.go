package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type sample struct {
	status  int
	bytes   int64
	latency time.Duration
	err     error
}

func run(cfg config) error {
	results := collectionlist.NewListWithCapacity[benchmarkResult](cfg.targets.Len())
	var runErr error
	cfg.targets.Range(func(_ int, currentTarget target) bool {
		currentResult, err := runSingleTarget(currentTarget, cfg)
		if err != nil {
			runErr = err
			return false
		}
		results.Add(currentResult)
		if _, err := fmt.Print(markdownResult(currentResult)); err != nil {
			runErr = fmt.Errorf("print result: %w", err)
			return false
		}
		return true
	})
	if runErr != nil {
		return runErr
	}
	return writeReports(cfg, newBenchmarkReport(cfg, results))
}

func runSingleTarget(currentTarget target, cfg config) (benchmarkResult, error) {
	if cfg.warmup > 0 {
		if _, err := runTarget(currentTarget, cfg, cfg.warmup, false); err != nil {
			return benchmarkResult{}, fmt.Errorf("warm up %s: %w", currentTarget.Name, err)
		}
	}
	currentResult, err := runTarget(currentTarget, cfg, cfg.duration, true)
	if err != nil {
		return benchmarkResult{}, fmt.Errorf("run %s: %w", currentTarget.Name, err)
	}
	return currentResult, nil
}

func runTarget(currentTarget target, cfg config, duration time.Duration, collect bool) (benchmarkResult, error) {
	transport := &http.Transport{
		DisableCompression:  true,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     cfg.concurrency * 4,
		MaxIdleConns:        cfg.concurrency * 4,
		MaxIdleConnsPerHost: cfg.concurrency * 4,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.timeout,
	}
	currentResult := newBenchmarkResult(currentTarget)
	samples := make(chan sample, cfg.concurrency*256)
	deadline := time.Now().Add(duration)
	started := time.Now()

	var workers sync.WaitGroup
	for range cfg.concurrency {
		workers.Go(func() {
			runWorker(deadline, client, cfg.method, currentTarget.URL, collect, samples)
		})
	}
	go func() {
		workers.Wait()
		close(samples)
	}()
	for sample := range samples {
		if collect {
			currentResult.add(sample)
		}
	}
	currentResult.DurationSeconds = time.Since(started).Seconds()
	currentResult.finalize()
	return currentResult, nil
}

func runWorker(
	deadline time.Time,
	client *http.Client,
	method string,
	targetURL string,
	collect bool,
	samples chan<- sample,
) {
	for time.Now().Before(deadline) {
		currentSample := executeRequest(client, method, targetURL)
		if collect {
			samples <- currentSample
		}
	}
}

func executeRequest(client *http.Client, method, targetURL string) sample {
	reqCtx, cancel := context.WithTimeout(context.Background(), client.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, targetURL, http.NoBody)
	if err != nil {
		return sample{err: err}
	}
	started := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(started)
	currentSample := sample{latency: elapsed, err: err}
	if resp == nil {
		return currentSample
	}
	currentSample.status = resp.StatusCode
	currentSample.bytes, currentSample.err = readResponse(resp, currentSample.err)
	return currentSample
}

func readResponse(resp *http.Response, requestErr error) (int64, error) {
	bytes, readErr := io.Copy(io.Discard, resp.Body)
	closeErr := resp.Body.Close()
	return bytes, errors.Join(requestErr, readErr, closeErr)
}

func newBenchmarkResult(currentTarget target) benchmarkResult {
	return benchmarkResult{
		Target:      currentTarget,
		StatusCodes: mapping.NewMap[string, uint64](),
		latencies:   collectionlist.NewList[time.Duration](),
	}
}

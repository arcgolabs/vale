package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
)

type benchmarkReport struct {
	Tool        string                                `json:"tool"`
	StartedAt   time.Time                             `json:"started_at"`
	Duration    string                                `json:"duration"`
	Warmup      string                                `json:"warmup"`
	Concurrency int                                   `json:"concurrency"`
	Timeout     string                                `json:"timeout"`
	Method      string                                `json:"method"`
	Path        string                                `json:"path"`
	Results     *collectionlist.List[benchmarkResult] `json:"results"`
}

type benchmarkResult struct {
	Target            target                       `json:"target"`
	DurationSeconds   float64                      `json:"duration_seconds"`
	Requests          uint64                       `json:"requests"`
	Errors            uint64                       `json:"errors"`
	Bytes             uint64                       `json:"bytes"`
	RequestsPerSecond float64                      `json:"requests_per_second"`
	BytesPerSecond    float64                      `json:"bytes_per_second"`
	StatusCodes       *mapping.Map[string, uint64] `json:"status_codes"`
	Latency           latencyStats                 `json:"latency"`

	latencies    *collectionlist.List[time.Duration]
	totalLatency time.Duration
}

type latencyStats struct {
	MinMs  float64 `json:"min_ms"`
	MeanMs float64 `json:"mean_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P90Ms  float64 `json:"p90_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	MaxMs  float64 `json:"max_ms"`
}

func (r *benchmarkResult) add(currentSample sample) {
	r.Requests++
	if currentSample.err != nil {
		r.Errors++
		return
	}
	if currentSample.bytes > 0 {
		r.Bytes += uint64(currentSample.bytes)
	}
	status := strconv.Itoa(currentSample.status)
	count, _ := r.StatusCodes.Get(status)
	r.StatusCodes.Set(status, count+1)
	r.latencies.Add(currentSample.latency)
	r.totalLatency += currentSample.latency
}

func (r *benchmarkResult) finalize() {
	if r.DurationSeconds > 0 {
		r.RequestsPerSecond = float64(r.Requests) / r.DurationSeconds
		r.BytesPerSecond = float64(r.Bytes) / r.DurationSeconds
	}
	if r.latencies.IsEmpty() {
		return
	}
	sorted := r.latencies.Clone().Sort(func(left, right time.Duration) int {
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
		return 0
	})
	first, _ := sorted.GetFirst()
	last, _ := sorted.GetLast()
	r.Latency = latencyStats{
		MinMs:  durationMillis(first),
		MeanMs: durationMillis(r.totalLatency / time.Duration(sorted.Len())),
		P50Ms:  percentileMillis(sorted, 50),
		P90Ms:  percentileMillis(sorted, 90),
		P95Ms:  percentileMillis(sorted, 95),
		P99Ms:  percentileMillis(sorted, 99),
		MaxMs:  durationMillis(last),
	}
	r.latencies = nil
}

func newBenchmarkReport(cfg config, results *collectionlist.List[benchmarkResult]) benchmarkReport {
	return benchmarkReport{
		Tool:        toolName,
		StartedAt:   time.Now().UTC(),
		Duration:    cfg.duration.String(),
		Warmup:      cfg.warmup.String(),
		Concurrency: cfg.concurrency,
		Timeout:     cfg.timeout.String(),
		Method:      cfg.method,
		Path:        cfg.path,
		Results:     results,
	}
}

func writeReports(cfg config, currentReport benchmarkReport) error {
	if cfg.jsonPath != "" {
		if err := writeJSONReport(cfg.jsonPath, currentReport); err != nil {
			return err
		}
	}
	if cfg.markdownPath != "" {
		if err := writeMarkdownReport(cfg.markdownPath, currentReport); err != nil {
			return err
		}
	}
	return nil
}

func percentileMillis(sorted *collectionlist.List[time.Duration], percentile int) float64 {
	if sorted == nil || sorted.IsEmpty() {
		return 0
	}
	index := ((sorted.Len() * percentile) + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > sorted.Len() {
		index = sorted.Len()
	}
	value, _ := sorted.Get(index - 1)
	return durationMillis(value)
}

func durationMillis(duration time.Duration) float64 {
	return float64(duration.Nanoseconds()) / float64(time.Millisecond)
}

func markdownResult(currentResult benchmarkResult) string {
	return fmt.Sprintf(
		"| %s | %.2f | %d | %d | %.3f | %.3f | %.3f | %.3f | %.3f | %s |\n",
		currentResult.Target.Name,
		currentResult.RequestsPerSecond,
		currentResult.Requests,
		currentResult.Errors,
		currentResult.Latency.MeanMs,
		currentResult.Latency.P50Ms,
		currentResult.Latency.P95Ms,
		currentResult.Latency.P99Ms,
		currentResult.Latency.MaxMs,
		formatStatusCodes(currentResult.StatusCodes),
	)
}

func writeJSONReport(path string, currentReport benchmarkReport) error {
	content, err := json.MarshalIndent(currentReport, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json report: %w", err)
	}
	return writeReportFile(path, append(content, '\n'))
}

func writeMarkdownReport(path string, currentReport benchmarkReport) error {
	lines := collectionlist.NewList[string](
		"# Proxy Benchmark\n\n",
		fmt.Sprintf("- tool: `%s`\n", currentReport.Tool),
		fmt.Sprintf("- started_at: `%s`\n", currentReport.StartedAt.Format(time.RFC3339)),
		fmt.Sprintf("- duration: `%s`\n", currentReport.Duration),
		fmt.Sprintf("- warmup: `%s`\n", currentReport.Warmup),
		fmt.Sprintf("- concurrency: `%d`\n", currentReport.Concurrency),
		fmt.Sprintf("- timeout: `%s`\n", currentReport.Timeout),
		fmt.Sprintf("- method: `%s`\n", currentReport.Method),
		fmt.Sprintf("- path: `%s`\n\n", currentReport.Path),
		"| target | req/s | requests | errors | mean ms | p50 ms | p95 ms | p99 ms | max ms | status |\n",
		"| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n",
	)
	currentReport.Results.Range(func(_ int, currentResult benchmarkResult) bool {
		lines.Add(markdownResult(currentResult))
		return true
	})
	return writeReportFile(path, []byte(lines.Join("")))
}

func writeReportFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}
	if err := os.WriteFile(filepath.Clean(path), content, 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}

func formatStatusCodes(statusCodes *mapping.Map[string, uint64]) string {
	if statusCodes == nil || statusCodes.IsEmpty() {
		return "-"
	}
	keys := collectionlist.NewList(statusCodes.Keys()...).Sort(strings.Compare)
	return keys.Join(", ", func(_ int, key string) string {
		count, _ := statusCodes.Get(key)
		return key + "=" + strconv.FormatUint(count, 10)
	})
}

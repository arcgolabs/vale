# Benchmarks

This directory keeps repeatable benchmark harnesses separate from normal unit
tests.

## Go Micro Benchmarks

Run internal hot-path benchmarks:

```bash
go test ./runtime -run '^$' -bench 'Benchmark(MatchRoute|GatewayHandler)' -benchmem
go test ./compiler -run '^$' -bench BenchmarkCompileByRouteCount -benchmem
```

These benchmarks cover compiled route matching, gateway handler overhead, and
config-to-runtime snapshot compilation.

## Proxy Comparison

The Docker Compose scenario compares Vale, Traefik, and Caddy against the same
upstream service. By default it uses the published Vale image from GitHub
Container Registry (`ghcr.io/arcgolabs/vale:latest`) so pressure tests exercise
the release binary container. Access logs and metrics are disabled where
possible so the numbers focus on reverse proxy overhead.

Default host ports:

- Vale: `http://127.0.0.1:18080`
- Traefik: `http://127.0.0.1:18081`
- Caddy: `http://127.0.0.1:18082`

Run on PowerShell:

```powershell
./benchmarks/bench-compare.ps1 -Duration 30s -Warmup 5s -Concurrency 64 -LogLevel info
```

Use a specific GitHub-published Vale image:

```powershell
./benchmarks/bench-compare.ps1 -ValeImage ghcr.io/arcgolabs/vale:v0.1.0
```

Use an already-built local image:

```powershell
./benchmarks/bench-compare.ps1 -ValeImage vale-upx-test:upx -SkipPull
```

Use a local source build instead:

```powershell
./benchmarks/bench-compare.ps1 -LocalBuild
```

Local source builds run UPX through the repository Dockerfile.

Run on POSIX shells:

```bash
DURATION=30s WARMUP=5s CONCURRENCY=64 LOG_LEVEL=info ./benchmarks/bench-compare.sh
```

Use a specific GitHub-published Vale image:

```bash
VALE_IMAGE=ghcr.io/arcgolabs/vale:v0.1.0 ./benchmarks/bench-compare.sh
```

Use an already-built local image:

```bash
VALE_IMAGE=vale-upx-test:upx SKIP_PULL=1 ./benchmarks/bench-compare.sh
```

Use a local source build instead:

```bash
LOCAL_BUILD=1 ./benchmarks/bench-compare.sh
```

Local source builds run UPX through the repository Dockerfile.

Benchmark progress logs are written to stderr. Use `-LogLevel off` on
PowerShell or `LOG_LEVEL=off` on POSIX shells to keep only the result table.

The scripts write:

- `benchmarks/results/<timestamp>/proxybench.md`
- `benchmarks/results/<timestamp>/proxybench.json`
- `benchmarks/results/<timestamp>/images.txt`

Image tags are intentionally configurable:

```bash
TRAEFIK_IMAGE=traefik:v3 CADDY_IMAGE=caddy:2-alpine ./benchmarks/bench-compare.sh
```

For release-quality numbers, run on a quiet Linux host, pin image tags or image
digests, record CPU limits, and repeat each benchmark enough times to compare
medians instead of a single run.

# Contract — Operational endpoints

**Feature**: Weather Cache Demo for eBPF Talk
**Date**: 2026-05-14

These endpoints exist to satisfy Constitution III (Observable by Default)
and to make the demo recoverable on stage.

---

## `GET /healthz` — Liveness probe

| Aspect           | Value                                                              |
|------------------|--------------------------------------------------------------------|
| Method           | `GET`                                                              |
| Auth             | none                                                               |
| Success response | `200 OK`, body: `ok\n`, `Content-Type: text/plain; charset=utf-8`  |
| Failure response | The endpoint never returns non-2xx by design.                       |
| Latency budget   | < 10 ms p99 (no I/O performed)                                      |
| Side effects     | none                                                                |

Semantics: returns 200 as long as the process can serve HTTP. It does
**not** verify Redis or Open-Meteo connectivity — that would create a
false dependency between liveness and external systems and could trigger
unwanted Pod restarts during the talk if Open-Meteo had a hiccup.

The Kubernetes Deployment uses this endpoint as both `livenessProbe` and
`readinessProbe`.

## `GET /metrics` — Prometheus text exposition

| Aspect           | Value                                                              |
|------------------|--------------------------------------------------------------------|
| Method           | `GET`                                                              |
| Auth             | none                                                               |
| Success response | `200 OK`, `Content-Type: text/plain; version=0.0.4; charset=utf-8` |
| Format           | Prometheus text exposition format, served by `promhttp.Handler()`  |

### Custom metrics exposed

| Metric                                  | Type      | Labels                       | Description                                                  |
|-----------------------------------------|-----------|------------------------------|--------------------------------------------------------------|
| `weather_requests_total`                | Counter   | `city`, `source`, `status`   | One increment per `/api/weather` request. `source` ∈ `cache|origin|bypass|error`. `status` is the HTTP status code as a string. |
| `weather_request_duration_seconds`      | Histogram | `city`, `source`             | Buckets: `0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8`.                 |
| `weather_origin_requests_total`         | Counter   | `city`, `outcome`            | Increments only when the app actually dials Open-Meteo. `outcome` ∈ `success|timeout|http_error|parse_error`. Useful for cross-checking against Hubble's external-flow count. |
| `weather_cache_operations_total`        | Counter   | `op`, `outcome`              | `op` ∈ `get|set`; `outcome` ∈ `hit|miss|error|corrupt`.      |

Plus the default Go runtime collectors (`go_gc_*`, `go_goroutines`,
`process_*`, `process_resident_memory_bytes`, etc.).

### Metric label cardinality

- `city` is bounded by the catalog size (≤ 32 in practice).
- `source` and `outcome` are small enums.
- Total time series stay well under 1000, safely below scraping concerns.

## What is intentionally NOT exposed

- **No tracing endpoint.** Adding OpenTelemetry would introduce an
  exporter and at least one outbound dial-out (constitution IV violation
  unless we ship a collector inside the cluster, which is out of scope).
- **No `/debug/pprof/`.** Easy to add later if needed; off by default to
  avoid an unintended source of egress (e.g. someone fetching a heap
  profile from outside the cluster mid-talk).
- **No remote logging.** Logs go to stdout; the cluster's existing logging
  stack (if any) handles them. No agent, no sidecar.

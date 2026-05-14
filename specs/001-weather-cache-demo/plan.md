# Implementation Plan: Weather Cache Demo for eBPF Talk

**Branch**: `001-weather-cache-demo` | **Date**: 2026-05-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-weather-cache-demo/spec.md`

## Summary

A single-binary Go web service that serves a server-rendered weather dashboard
for ~8 South-American cities, backed by an in-cluster Redis cache. For most
cities the app reads/writes Redis with a 1-hour TTL; for cities listed in
`BYPASS_CITIES` (default: `sao-paulo`) the app skips the cache and always
calls the external provider (Open-Meteo, no API key).

The whole app — HTML, JSON API, cache logic, weather client, metrics —
runs in **one process / one Pod**, so during the eBPF talk every packet
leaving the Pod is unambiguously attributable to the cache-bypass policy.
The companion deliverables are a Dockerfile, a Makefile, a small loadgen
script, and Kubernetes manifests targeting an existing kind cluster with
MetalLB.

## Technical Context

**Language/Version**: Go 1.22+ (uses `log/slog`, `net/http` ServeMux patterns)

**Primary Dependencies**:

- `github.com/redis/go-redis/v9` — Redis client.
- `github.com/prometheus/client_golang/prometheus` + `promhttp` — Prometheus
  text endpoint at `/metrics`.
- Standard library only for HTTP server (`net/http`), HTML templates
  (`html/template`), structured logs (`log/slog`), JSON
  (`encoding/json`), HTTP client (`net/http` + `context`).
- Test deps: `github.com/alicebob/miniredis/v2` (in-process Redis fake),
  `github.com/stretchr/testify` (assertions only — optional).
- **No web framework**, **no ORM**, **no build tools beyond `go build`**.

**Storage**: Redis 7-alpine, in-cluster `Deployment` (no PVC, no persistence).
Key schema: `weather:v1:<slug>` → JSON-encoded `WeatherSnapshot`, TTL via
Redis `EX` set on write.

**Testing**:

- Unit tests: `go test ./...`
- Cache decision logic + Open-Meteo client parser → 100% covered (small).
- Handlers exercised with `httptest.NewRecorder` + `miniredis`.
- No integration tests against a live kind cluster; the quickstart doubles
  as a manual smoke test.

**Target Platform**: Linux/amd64 container, deployed on a kind cluster
running on the speaker's Ubuntu host. Image base: `gcr.io/distroless/static`
(static binary, no shell, no libc dependencies).

**Project Type**: Single web service (`Option 1: Single project` from the
template). HTML, JSON, and metrics all served from the same binary.

**Performance Goals** (informed by spec SC-006):

- Cache hit p95 < 100 ms (intra-Pod Redis call dominates).
- Cache miss p95 < 2 s (bounded by Open-Meteo response time).
- Sustained demo throughput: ≥ 10 req/s for the loadgen window — well
  within the budget of a single-replica Pod on a laptop-grade host.

**Constraints**:

- No secrets, no API keys, no auth (constitution II).
- No outbound HTTPS other than Open-Meteo + intra-cluster Redis
  (constitution IV).
- Single binary; no sidecars in the app Pod (constitution I).
- Image built locally and distributed via `kind load docker-image`; no
  external registry.

**Scale/Scope**:

- 8 cities by default (configurable via ConfigMap).
- 1 replica of the app, 1 replica of Redis.
- Code budget: ≤ 600 LOC Go + ≤ 150 LOC HTML/CSS + ≤ 200 LOC YAML.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution: `.specify/memory/constitution.md` v1.0.0

| Gate | Status | Notes |
|------|--------|-------|
| **I. Single Binary, Single Process** (NON-NEGOTIABLE) | ✅ PASS | Plan describes one Go binary; Dockerfile produces a single distroless image; no sidecars in app Pod (Redis is a separate Pod, not a sidecar). |
| **II. Demo-Friendly Reproducibility** (NON-NEGOTIABLE) | ✅ PASS | All config via ConfigMap/env. Open-Meteo requires no API key. Image distributed via `kind load`. No PVC, no Secret in any manifest. |
| **III. Observable by Default** | ✅ PASS | `source` field in both UI badge and JSON; `slog` JSON handler; `/healthz`; `/metrics` exposes `weather_requests_total{city,source}` and latency histogram. |
| **IV. eBPF Narrative Integrity** (NON-NEGOTIABLE) | ✅ PASS | Cache code path explicitly skipped for cities in `BYPASS_CITIES`. No background goroutines that perform I/O. No retries on cache hits. No telemetry beacons. The only outbound dial-out registered is `api.open-meteo.com:443`. |

**Re-check after Phase 1** (post-design): see [Post-Design Re-evaluation](#post-design-re-evaluation) at the bottom.

**No violations. No Complexity Tracking entries needed.**

## Project Structure

### Documentation (this feature)

```text
specs/001-weather-cache-demo/
├── plan.md              # This file (/speckit-plan output)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── http-api.md      #   GET /api/weather contract (request/response shape)
│   ├── ui.md            #   GET / template variables + badge contract
│   └── ops.md           #   /healthz, /metrics contract
├── spec.md              # /speckit-specify output
├── checklists/
│   └── requirements.md  # /speckit-specify output
└── tasks.md             # Phase 2 output (/speckit-tasks - NOT created here)
```

### Source Code (repository root)

```text
weather-app/
├── cmd/
│   └── weather-app/
│       └── main.go                  # Entrypoint: load config, wire deps, ListenAndServe
├── internal/
│   ├── config/
│   │   ├── config.go                # Env-var loader, defaults, validation
│   │   └── cities.go                # Default city catalog (slug, name, lat, lon)
│   ├── weather/
│   │   ├── client.go                # Open-Meteo HTTP client
│   │   └── client_test.go
│   ├── cache/
│   │   ├── redis.go                 # Get/Set wrappers around go-redis
│   │   └── redis_test.go            # uses miniredis
│   ├── server/
│   │   ├── handlers.go              # /, /api/weather, /healthz
│   │   ├── handlers_test.go
│   │   ├── decide.go                # cache-vs-origin decision (pure function, easy to test)
│   │   ├── decide_test.go
│   │   ├── metrics.go               # Prometheus collectors
│   │   └── routes.go                # http.ServeMux wiring
│   └── obs/
│       └── log.go                   # slog JSON handler setup
├── web/
│   ├── templates/
│   │   └── index.html               # Single-page UI with city dropdown + result + badge
│   └── static/
│       └── styles.css               # Big readable badge for projector
├── manifests/
│   ├── namespace.yaml
│   ├── configmap.yaml               # Cities list, BYPASS_CITIES, TTL_SECONDS, REDIS_ADDR
│   ├── redis-deployment.yaml
│   ├── redis-service.yaml
│   ├── weather-app-deployment.yaml
│   ├── weather-app-service.yaml     # type: LoadBalancer (MetalLB)
│   └── kustomization.yaml           # convenience apply
├── loadgen/
│   ├── loadgen.sh                   # bash + curl, weighted 90/10 by default
│   └── loadgen.yaml                 # optional in-cluster Job (so Hubble sees pod-to-pod)
├── Dockerfile                       # multi-stage: builder + distroless
├── Makefile                         # build, image, load, deploy, undeploy, loadgen, test
├── go.mod
├── go.sum
├── .dockerignore
├── README.md
└── LICENSE
```

**Structure Decision**: Single Go service following the
`cmd/<binary>` + `internal/<package>` idiom. UI assets live in `web/`.
Deployment artifacts (Dockerfile, Makefile, manifests, loadgen) live at
the repo root. This is "Option 1: Single project" from the template,
specialized for Go.

## Phase 0 — Outline & Research

**Output**: see [`research.md`](./research.md). Key decisions resolved:

- External provider: **Open-Meteo** (no key, free for the talk's volume).
- Endpoint shape: `https://api.open-meteo.com/v1/forecast` with `current_weather=true`.
- Coordinates baked into the city catalog (no geocoding hop needed).
- Redis key naming and TTL strategy.
- HTTP framework: **stdlib only** (`net/http.ServeMux`).
- Logging: **`log/slog`** with the JSON handler.
- Image base: **`gcr.io/distroless/static-debian12:nonroot`**.
- Image distribution: `kind load docker-image` (cluster name default `kind`).

## Phase 1 — Design & Contracts

**Output**:

- [`data-model.md`](./data-model.md) — `City`, `WeatherSnapshot`, `BypassPolicy` definitions, validation rules, Redis key schema.
- [`contracts/http-api.md`](./contracts/http-api.md) — `GET /api/weather` request/response.
- [`contracts/ui.md`](./contracts/ui.md) — template data model and badge rendering rule.
- [`contracts/ops.md`](./contracts/ops.md) — `/healthz`, `/metrics`.
- [`quickstart.md`](./quickstart.md) — exact commands (build → load → apply → access → loadgen → reset → undeploy).
- [`.cursor/rules/specify-rules.mdc`](../../.cursor/rules/specify-rules.mdc) — updated to reference this plan.

## Post-Design Re-evaluation

Re-checking the constitution gates against the actual designed artifacts (data model, contracts, quickstart):

| Gate | Status | Evidence |
|------|--------|----------|
| **I. Single Binary, Single Process** | ✅ PASS | `data-model.md` shows no separate front/back; `contracts/ui.md` is rendered by the same handler that exposes `/api/weather`. |
| **II. Demo-Friendly Reproducibility** | ✅ PASS | `quickstart.md` lists 4 commands to go from clean cluster to demo. No Secret resource appears in `manifests/`. |
| **III. Observable by Default** | ✅ PASS | `contracts/ops.md` specifies `weather_requests_total{city,source}` and `weather_request_duration_seconds`. JSON contract in `contracts/http-api.md` includes `source`. |
| **IV. eBPF Narrative Integrity** | ✅ PASS | `data-model.md` defines `BypassPolicy` as immutable post-startup. The decision pseudocode in `contracts/http-api.md` shows the bypass branch reads `nil` from cache and writes nothing. No background tickers in the design. |

No new violations introduced by the design phase. Plan ready for `/speckit-tasks`.

## Complexity Tracking

> No constitutional violations. Section intentionally empty.

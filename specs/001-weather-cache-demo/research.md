# Phase 0 — Research

**Feature**: Weather Cache Demo for eBPF Talk
**Date**: 2026-05-14

This document records the technical decisions made before design. Every entry
follows the format **Decision / Rationale / Alternatives considered**. There
are no `NEEDS CLARIFICATION` markers left at the end of this phase.

---

## R-001 — External weather provider

**Decision**: [Open-Meteo](https://open-meteo.com/), endpoint
`https://api.open-meteo.com/v1/forecast`, query parameters
`?latitude=<lat>&longitude=<lon>&current_weather=true&timezone=auto`.

**Rationale**:

- No API key, no signup, no rate-limit headaches for an 8-city demo.
- Public DNS (`api.open-meteo.com` → CloudFront), so it shows up cleanly
  in Hubble as a non-cluster destination, which is exactly what the
  audience needs to see during the eBPF segment.
- Stable JSON shape with a `current_weather` block that already contains
  `temperature` (°C), `weathercode`, and `time` — no second call needed.
- Free tier permits ~10 000 requests/day, far above the demo budget
  (≈ 8 cities × 1 refresh/hour + bypass cities ≈ trivial).

**Alternatives considered**:

- **OpenWeatherMap**: requires an API key. Forces us to manage a Kubernetes
  `Secret`, which would violate Constitution II (Demo-Friendly
  Reproducibility) — explicitly out.
- **wttr.in**: zero-config but returns text/HTML and has occasional
  rate-limit responses; harder to parse deterministically.
- **WeatherAPI.com**: free key but still requires the key.

---

## R-002 — Open-Meteo response parsing

**Decision**: Decode only the `current_weather` sub-object and the
`current_weather.temperature` and `current_weather.weathercode` fields.
Map `weathercode` to a short human-readable string via a static table
([WMO weather interpretation codes](https://open-meteo.com/en/docs)).

**Rationale**:

- Fewer fields = smaller, easier-to-test struct.
- The mapping table is short (~15 entries) and lives in the `weather`
  package as a private `var codeToCondition map[int]string`.
- `weathercode` is already an integer, no string normalization needed.

**Alternatives considered**:

- Parsing the full forecast block (hourly arrays): unnecessary, much more
  data to validate, and not displayed in the UI.
- Calling Open-Meteo's "weather code descriptions" endpoint: extra
  external call, defeats Constitution IV (eBPF Narrative Integrity).

---

## R-003 — City catalog and coordinates

**Decision**: A static `[]City` slice baked into the binary as a default,
overridable via the `CITIES` environment variable (JSON array). Default
catalog (8 cities, all in South America):

| Slug           | Display Name   | Latitude  | Longitude |
|----------------|----------------|-----------|-----------|
| `bogota`       | Bogotá         |   4.7110  |  -74.0721 |
| `lima`         | Lima           | -12.0464  |  -77.0428 |
| `santiago`     | Santiago       | -33.4489  |  -70.6693 |
| `buenos-aires` | Buenos Aires   | -34.6037  |  -58.3816 |
| `caracas`      | Caracas        |  10.4806  |  -66.9036 |
| `quito`        | Quito          |  -0.1807  |  -78.4678 |
| `montevideo`   | Montevideo     | -34.9011  |  -56.1645 |
| `sao-paulo`    | São Paulo      | -23.5505  |  -46.6333 |

`BYPASS_CITIES` defaults to `sao-paulo`.

**Rationale**:

- 8 cities is enough to make the dropdown look real but small enough to
  keep the demo focused.
- Coordinates baked in avoid a geocoding hop (one less external dial-out,
  protecting Constitution IV).
- Slugs are URL-safe and used as both Redis key suffix and HTTP query
  parameter — no normalization at request time.

**Alternatives considered**:

- Reading from a YAML file mounted in the ConfigMap: more flexible but
  introduces a parsing step that can fail at startup. Env var with JSON
  default + slice constant gives the same flexibility with less surface.
- Geocoding by city name at runtime: extra external call, breaks IV.

---

## R-004 — Redis client choice

**Decision**: `github.com/redis/go-redis/v9` (the official client).

**Rationale**:

- De-facto standard for Go + Redis in 2026; well-maintained.
- Built-in support for `context.Context` cancellation/deadlines.
- Plays nicely with `miniredis` for unit tests.

**Alternatives considered**:

- `gomodule/redigo`: lower-level, no native context support in the same way.
- Hand-rolled RESP client: way too much yak-shaving for a demo.

---

## R-005 — Redis cache key schema and TTL semantics

**Decision**:

- Key: `weather:v1:<slug>` (e.g. `weather:v1:bogota`).
- Value: JSON-serialized `WeatherSnapshot` (see `data-model.md`).
- TTL: set on every `SET` via `EX TTL_SECONDS` (default 3600).
- Reads: `GET <key>`. If the key is absent → cache miss. If `Unmarshal`
  fails → treat as cache miss and overwrite with a fresh entry.
- Writes: only by the cache-miss branch and only for non-bypass cities.

**Rationale**:

- The `v1:` prefix lets us bump the schema later without colliding with
  stale entries from a previous demo.
- Atomic TTL on `SET` is simpler than `EXPIRE` after the fact and avoids
  the race window where a key exists without an expiry.
- Treating Unmarshal failure as a miss is the safest behavior for a
  demo: the worst outcome is one extra origin call, not a crash on stage.

**Alternatives considered**:

- Hash-based storage (`HSET`): unnecessary; we always read/write the
  whole snapshot.
- Lua scripts for atomic check-and-set: overkill for the demo's
  consistency requirements (the spec explicitly tolerates double
  origin calls during a race; see edge case #5).

---

## R-006 — HTTP framework

**Decision**: Standard library `net/http`, using Go 1.22+ pattern routing
(`mux.HandleFunc("GET /api/weather", …)`).

**Rationale**:

- Go 1.22's enhanced ServeMux supports method+path patterns natively, so
  there's no functional gap with chi/echo/gin for this app's 4 routes.
- One less dependency in `go.mod` reduces image size and supply-chain
  surface, both nice talking points for an SRE-flavored demo.

**Alternatives considered**:

- `chi`: lovely router, but unjustified for 4 routes.
- `gin`: brings JSON helpers we don't need; pulls in reflect-heavy paths.
- `echo`: same observation as gin.

---

## R-007 — Structured logging

**Decision**: `log/slog` from the standard library, with `slog.NewJSONHandler`
writing to `os.Stdout` at `LevelInfo` by default. Log fields per request:
`ts`, `level`, `msg`, `city`, `source`, `status_code`, `latency_ms`,
`request_id`.

**Rationale**:

- Stdlib, no extra dep.
- JSON to stdout is the canonical "Kubernetes-native" log format; the
  cluster's existing log aggregator (if any) just works.
- `request_id` is a tiny addition (random 64-bit hex) but pays off when
  correlating eBPF flow logs with app logs during the talk.

**Alternatives considered**:

- `zap`: faster, but allocations are a non-issue at 10 req/s and slog is
  perfectly idiomatic in 2026.
- `zerolog`: same observation.

---

## R-008 — Metrics

**Decision**: `prometheus/client_golang` exposing two collectors at `/metrics`:

- `weather_requests_total{city,source,status}` (Counter) — `source` is
  one of `cache|origin|bypass|error`.
- `weather_request_duration_seconds{city,source}` (Histogram) — buckets
  `0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8`.

Plus the default Go process collectors (`go_*`, `process_*`) registered by
`prometheus.NewRegistry()`.

**Rationale**:

- Two metrics are enough for the SC-004 measurement ("≥ 95 % cache hits
  once warmed") and to cross-check Hubble's external-flow count against
  `weather_requests_total{source="bypass"}` during the talk.
- Prom client is mature, low-overhead, and the de-facto standard.

**Alternatives considered**:

- OpenTelemetry SDK: overkill; would pull in a metrics exporter and likely
  introduce its own outbound dial-out, violating IV.
- StatsD: requires an external collector; adds a Pod, breaks I.

---

## R-009 — Container image

**Decision**: Multi-stage `Dockerfile`:

1. `golang:1.22-bookworm` as builder, runs `CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/weather-app ./cmd/weather-app`.
2. Runtime: `gcr.io/distroless/static-debian12:nonroot`. Copies the binary
   and the `web/` directory.

Image tag: `weather-app:dev` (no registry).

**Rationale**:

- Distroless static + CGO_ENABLED=0 → ~15 MB image, no shell to confuse
  the eBPF capture story (no `curl`/`wget` shipped in the Pod).
- `nonroot` user satisfies any future PSP/PSS hardening without extra
  work.
- `:dev` is an explicit "this is a demo build" tag.

**Alternatives considered**:

- `alpine`: small but ships busybox; one more thing to explain on stage.
- `scratch`: requires shipping CA certs ourselves (we need to talk
  HTTPS to Open-Meteo). Distroless ships them already.

---

## R-010 — Image distribution to kind

**Decision**: `kind load docker-image weather-app:dev --name <KIND_CLUSTER>`
where `KIND_CLUSTER` defaults to `kind` and is overridable via Makefile var.

**Rationale**:

- The user has an existing kind cluster on Linux. `kind load` injects the
  image directly into all nodes' containerd; no registry needed.
- Setting `imagePullPolicy: IfNotPresent` in the Deployment ensures
  Kubernetes uses the loaded image and doesn't try to pull from
  Docker Hub.

**Alternatives considered**:

- Local registry on `localhost:5001` mirrored into kind: more moving
  parts, more failure modes during a live demo.
- Pushing to Docker Hub or GHCR: requires credentials, network egress
  from the talk venue, and a public image — overkill.

---

## R-011 — Service exposure

**Decision**: `Service` of `type: LoadBalancer` with no extra annotations.
MetalLB's IP pool will assign an IP; the operator finds it with
`kubectl get svc -n weather-demo weather-app`.

**Rationale**:

- The user already has MetalLB installed; this is the cleanest UX.
- LAN-reachable IP makes User Story 4 (audience self-validates) trivial.
- Hubble flow visualization is more readable when the ingress hop is a
  single LB IP rather than a NodePort spread across nodes.

**Alternatives considered**:

- `NodePort`: works without MetalLB but requires the audience to know a
  node IP and a high port — clunky.
- `Ingress`: needs an ingress controller; one more component to explain
  before the eBPF segment.

---

## R-012 — Load generator

**Decision**: A bash script `loadgen/loadgen.sh` using `curl` in a loop with
a weighted random pick. Default split: 90 % non-bypass cities (uniformly
distributed among them), 10 % bypass city. Defaults: 30 req/s for 60 s,
both overridable via env vars / flags. Optionally wrapped in a Kubernetes
`Job` (`loadgen/loadgen.yaml`) so the traffic originates from inside the
cluster (visible as pod-to-pod flows in Hubble).

**Rationale**:

- Bash + curl needs zero extra tooling on the speaker's host.
- The K8s Job variant makes the demo more visually compelling because the
  loadgen Pod's flows show up in Hubble alongside the app Pod's flows.

**Alternatives considered**:

- `hey`, `wrk`, `vegeta`, `k6`: more powerful but add a binary the speaker
  has to install. Not worth it for a 30-line script.

---

## R-013 — Repository layout

**Decision**: `cmd/weather-app/` + `internal/` + `web/` + `manifests/` +
`loadgen/`. See `plan.md` § Project Structure for the full tree.

**Rationale**:

- `cmd/<binary>` + `internal/` is the de-facto Go layout (popularized by
  golang-standards/project-layout); reviewers don't need explanation.
- `internal/` makes every package non-importable from outside the module,
  matching the "demo, not library" intent.

**Alternatives considered**:

- Flat layout (everything in package `main`): fine for a 200-LOC script
  but hurts testability of the cache-decision logic.

---

## Open questions resolved during this phase

None. All `NEEDS CLARIFICATION` markers from the spec template were already
addressed in the constitution and the spec; no follow-ups remain.

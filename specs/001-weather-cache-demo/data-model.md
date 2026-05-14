# Phase 1 — Data Model

**Feature**: Weather Cache Demo for eBPF Talk
**Date**: 2026-05-14

This document defines the entities, their fields, validation rules, and
storage representation. The application is small enough that all entities
live in process memory or in Redis; there is no relational store.

---

## Entity: City

A configured destination the demo serves weather for. The catalog is
**immutable at runtime** — loaded once from configuration at startup and
not mutated thereafter.

### Fields

| Field          | Type      | Constraints                                         | Notes                                |
|----------------|-----------|-----------------------------------------------------|--------------------------------------|
| `Slug`         | `string`  | non-empty, matches `^[a-z][a-z0-9-]*[a-z0-9]$`      | URL-safe identifier; primary key.    |
| `DisplayName`  | `string`  | non-empty, ≤ 64 chars                               | Shown in the UI dropdown.            |
| `Latitude`     | `float64` | `-90.0 ≤ Latitude ≤ 90.0`                           | Decimal degrees.                     |
| `Longitude`    | `float64` | `-180.0 ≤ Longitude ≤ 180.0`                        | Decimal degrees.                     |

### Validation rules

- The catalog MUST contain at least one city.
- Slugs MUST be unique within the catalog (case-sensitive equality).
- A city referenced in `BYPASS_CITIES` that does not exist in the catalog
  MUST cause the application to exit non-zero at startup with a clear log
  line — never silently ignored.

### Relationships

- A `City` owns zero or one `WeatherSnapshot` at any moment in Redis
  (only when it is **not** in the bypass policy).

### Source of truth

- Default catalog hard-coded in `internal/config/cities.go` (see
  `research.md` § R-003).
- Overridable at startup via the `CITIES` environment variable, which
  contains a JSON array of `{slug,name,lat,lon}` objects.

---

## Entity: WeatherSnapshot

The result of a weather lookup at a specific point in time, ready to render
in the UI and to serialize as JSON.

### Fields

| Field           | Type      | Constraints                                      | Notes                                            |
|-----------------|-----------|--------------------------------------------------|--------------------------------------------------|
| `City`          | `string`  | matches an existing `City.Slug`                  | Foreign-key-style reference.                     |
| `TemperatureC`  | `float64` | `-100.0 ≤ TemperatureC ≤ 100.0`                  | °C, as returned by Open-Meteo.                   |
| `Condition`     | `string`  | non-empty, ≤ 64 chars                            | Human-readable; mapped from WMO `weathercode`.   |
| `WeatherCode`   | `int`     | `0 ≤ WeatherCode ≤ 99`                           | Raw WMO code; preserved for traceability.        |
| `FetchedAt`     | `time.Time` (UTC) | not zero; not in the future by > 1 minute (clock skew tolerance) | When the snapshot was obtained from origin. |
| `Source`        | `string`  | one of `"cache"`, `"origin"`, `"bypass"`         | NOT persisted — set per response (see below).    |

### Lifecycle / state transitions

```
                       ┌────────────────────┐
   request comes in ──▶│ city in BYPASS?    │── yes ──▶ origin fetch ──▶ Source="bypass"
                       └────────────────────┘                            (NOT cached)
                                │ no
                                ▼
                       ┌────────────────────┐
                       │ Redis GET hit?     │── yes ──▶ Source="cache"
                       └────────────────────┘
                                │ no
                                ▼
                          origin fetch
                                │
                                ▼
                          Redis SET EX TTL
                                │
                                ▼
                          Source="origin"
```

### Persistence representation

- **Wire format (Redis value)** — JSON object, fields ONLY:
  `{"city": "...", "temperature_c": ..., "condition": "...", "weather_code": ..., "fetched_at": "RFC3339"}`.
  The `source` field is **never persisted** (only attached to the HTTP
  response just before send), which is what makes the "cached vs origin"
  distinction meaningful.

- **Wire format (HTTP response)** — same fields **plus** `source`. See
  `contracts/http-api.md`.

### Validation rules

- A snapshot read from Redis whose `FetchedAt` is older than `now - TTL`
  MUST be treated as expired. (Defense in depth — Redis already evicts
  via EX, but if Redis is restarted into a previously persisted dataset,
  this guard prevents serving stale data.)
- A snapshot whose JSON fails to unmarshal MUST be discarded and the
  request fall through to origin (logged as a `cache_corrupt` event).

### Concurrency

- The spec accepts a thundering herd on the first request to a cold key
  (edge case #5). No singleflight or distributed lock is required.
- Writes are last-writer-wins; for a 1-hour TTL on a demo, this is fine.

---

## Entity: BypassPolicy

The set of city slugs that MUST always be served from origin, never from
cache.

### Fields

| Field       | Type       | Constraints                                      | Notes                                |
|-------------|------------|--------------------------------------------------|--------------------------------------|
| `Cities`    | `set[str]` | each entry MUST be a valid `City.Slug`           | Loaded from `BYPASS_CITIES` env var. |

### Validation rules

- Default: `{"sao-paulo"}`.
- Empty set is allowed (means "no city bypasses cache"; observable
  behavior: zero external traffic after warm-up — useful for testing).
- All entries MUST also exist in the `City` catalog at startup.
- Immutable after process start.

### Relationships

- Read by the cache-decision function in `internal/server/decide.go` for
  every request.

---

## Configuration entity (not persisted; for completeness)

The runtime configuration aggregates everything above plus a few knobs:

| Field            | Source              | Default                               | Notes                                                |
|------------------|---------------------|---------------------------------------|------------------------------------------------------|
| `Cities`         | `CITIES` env (JSON) | 8 cities, see R-003                   | List of `City`.                                      |
| `BypassCities`   | `BYPASS_CITIES` env | `sao-paulo`                           | Comma-separated slugs.                               |
| `TTLSeconds`     | `TTL_SECONDS` env   | `3600`                                | Applied to Redis SET on every miss.                  |
| `RedisAddr`      | `REDIS_ADDR` env    | `redis:6379`                          | host:port.                                           |
| `OpenMeteoURL`   | `OPEN_METEO_URL` env| `https://api.open-meteo.com/v1/forecast` | Configurable for testing.                       |
| `ListenAddr`     | `LISTEN_ADDR` env   | `:8080`                               | Server bind.                                         |
| `LogLevel`       | `LOG_LEVEL` env     | `info`                                | `debug`/`info`/`warn`/`error`.                       |

All values are validated at startup; any failure exits non-zero with a
single structured log line — easier to diagnose than a panic.

---

## Out-of-scope entities (explicitly NOT modeled)

- **User** / authentication: the demo runs on a private LAN with no auth.
- **Forecast history** beyond the latest snapshot.
- **Audit log** of cache decisions: covered by structured request logs +
  Prometheus counters; no separate persisted entity is needed.

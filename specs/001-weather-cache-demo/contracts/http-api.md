# Contract — JSON API: `GET /api/weather`

**Feature**: Weather Cache Demo for eBPF Talk
**Date**: 2026-05-14

This contract is consumed by:

- The dashboard's client-side code (single `fetch` call per query).
- The load generator (`loadgen/loadgen.sh`).
- Any audience member who wants to `curl` the API during the talk.

---

## Endpoint

```
GET /api/weather?city=<slug>
```

- Method: `GET` (idempotent from the client's perspective; cache state may
  change as a side effect, but the response shape is deterministic).
- Path: exactly `/api/weather`.
- Query parameter: `city` — REQUIRED, must be a slug present in the
  configured catalog.
- Authentication: none.

## Request

| Field   | Where     | Type     | Required | Validation                                       |
|---------|-----------|----------|----------|--------------------------------------------------|
| `city`  | querystring | string | yes      | matches `^[a-z][a-z0-9-]*[a-z0-9]$` and exists in catalog |

No request body. No required headers.

## Successful response — `200 OK`

`Content-Type: application/json; charset=utf-8`

```json
{
  "city": "bogota",
  "display_name": "Bogotá",
  "temperature_c": 14.2,
  "condition": "Partly cloudy",
  "weather_code": 2,
  "source": "cache",
  "fetched_at": "2026-05-14T15:42:11Z"
}
```

| Field           | Type     | Notes                                                        |
|-----------------|----------|--------------------------------------------------------------|
| `city`          | string   | Echoes the slug from the request.                            |
| `display_name`  | string   | The configured human-readable name.                          |
| `temperature_c` | number   | °C. Always present.                                          |
| `condition`     | string   | Mapped from `weather_code` (e.g. `"Clear sky"`).             |
| `weather_code`  | integer  | Raw WMO code, 0–99.                                          |
| `source`        | enum     | One of `"cache"`, `"origin"`, `"bypass"`. **Always present.** |
| `fetched_at`    | string   | RFC 3339 / ISO 8601 in UTC. Time the snapshot left origin.   |

The `source` value is the canonical signal the demo's narrative depends on;
it MUST always be one of the three enum values and MUST never be omitted,
even on degraded paths (e.g. when Redis is unreachable, the field is
`"origin"` because the request had to go to Open-Meteo).

## Error responses

All errors use the same JSON envelope:

```json
{
  "error": "<machine_code>",
  "message": "<human_readable>"
}
```

| Status | `error` code           | When                                                                                  |
|--------|------------------------|---------------------------------------------------------------------------------------|
| `400`  | `missing_city`         | The `city` query parameter is absent or empty.                                        |
| `400`  | `invalid_city`         | `city` is present but does not match the slug regex.                                  |
| `404`  | `unknown_city`         | `city` is well-formed but not in the configured catalog.                              |
| `502`  | `origin_unavailable`   | Open-Meteo did not respond within the timeout (see below) or returned non-2xx.         |
| `503`  | `dependency_error`     | Both Redis and the origin failed (terminal degradation).                              |
| `405`  | `method_not_allowed`   | Any non-GET method on this path. Response includes `Allow: GET`.                      |

Errors MUST NOT trigger an external HTTPS request (FR-011 / spec edge
case "Unknown city in URL").

## Timeouts and retries

- Open-Meteo HTTP client timeout: **3 seconds** (`http.Client.Timeout`).
- Redis operation context deadline: **300 ms** for both `GET` and
  `SET EX`.
- **No retries.** A single failure propagates to the user and to logs/metrics.
  (Retries would muddy the eBPF capture; Constitution IV.)

## Caching behavior (decision pseudocode)

This is the source of truth for how `source` is computed:

```text
function handle(city):
    if not catalog.contains(city):
        return 404 unknown_city                             # zero external calls

    if bypass.contains(city):
        snap = origin.fetch(city)                           # ← only outbound dial-out
        if snap.err:
            return 502 origin_unavailable
        snap.source = "bypass"
        return 200 snap

    cached = redis.get("weather:v1:" + city)
    if cached.ok and cached.fresh and cached.unmarshal_ok:
        cached.source = "cache"
        return 200 cached

    snap = origin.fetch(city)                               # ← only outbound dial-out
    if snap.err:
        return 502 origin_unavailable
    redis.set("weather:v1:" + city, snap, ex=TTL)           # best effort; failure logged but not fatal
    snap.source = "origin"
    return 200 snap
```

Side-effect summary:

| Path                | Reads Redis | Writes Redis | Calls Open-Meteo |
|---------------------|-------------|--------------|------------------|
| Unknown city        | no          | no           | **no**           |
| Bypass city         | no          | no           | yes              |
| Non-bypass cache hit| yes         | no           | no               |
| Non-bypass cache miss| yes        | yes          | yes              |

This table is the contract that lets us claim, during the talk, that "the
only external traffic visible in Hubble is bypass-city traffic".

## Headers

The response always includes:

- `Content-Type: application/json; charset=utf-8`
- `X-Source: cache | origin | bypass` (mirrors the JSON `source` field —
  useful when piping `curl` through `grep` during a live demo).
- `X-Request-ID: <16-hex-char>` (also present in logs).

## Examples

### Cache hit

```
$ curl -sS "http://<lb-ip>/api/weather?city=bogota" | jq
{
  "city": "bogota",
  "display_name": "Bogotá",
  "temperature_c": 14.2,
  "condition": "Partly cloudy",
  "weather_code": 2,
  "source": "cache",
  "fetched_at": "2026-05-14T15:42:11Z"
}
```

### Bypass path (always external)

```
$ curl -sS "http://<lb-ip>/api/weather?city=sao-paulo" | jq .source
"bypass"
```

### Unknown city (no external call)

```
$ curl -sS -w "%{http_code}\n" "http://<lb-ip>/api/weather?city=atlantis"
{"error":"unknown_city","message":"city 'atlantis' not in catalog"}
404
```

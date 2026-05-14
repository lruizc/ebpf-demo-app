# Feature Specification: Weather Cache Demo for eBPF Talk

**Feature Branch**: `001-weather-cache-demo`

**Created**: 2026-05-14

**Status**: Draft

**Input**: User description: "Demo app for an eBPF talk. South-American weather
dashboard that caches results in a local Redis for one hour. One city is
deliberately configured to bypass the cache so the audience can observe — via
eBPF/Hubble at the kernel level — a constant trickle of external HTTPS traffic
while the rest of the cities serve from internal cache. Single binary, no
front-end/back-end split, deployed on an existing kind cluster with MetalLB."

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Speaker demonstrates cache vs. origin in the UI (Priority: P1)

The speaker opens the app's web UI on screen, picks a Suramerican city from a
dropdown (e.g. Bogotá), and clicks "Consultar". The page shows the current
temperature/condition and a clearly visible badge saying `cache HIT (internal)`
or `cache MISS (external)`. The speaker repeats the action with São Paulo and
the audience sees `external` every single time, while every other city flips
to `internal` after the first request.

**Why this priority**: This is the visual hook of the talk. Before any kernel
tooling appears on screen, the audience must understand the application
behavior with their own eyes. If this story works, the rest of the demo
(eBPF capture) is just confirmation; if it does not, no amount of Hubble
output will rescue the narrative.

**Independent Test**: Deploy the app + Redis to the kind cluster, open the
LoadBalancer URL in a browser, and consult each city twice. The badge MUST
read `external` then `internal` for every city except São Paulo, which MUST
read `external` both times.

**Acceptance Scenarios**:

1. **Given** an empty cache and a non-bypass city (e.g. Bogotá), **When** the
   speaker requests its weather for the first time, **Then** the response
   shows the current weather and a `external` (cache MISS) source indicator.
2. **Given** a non-bypass city was just consulted within the last hour,
   **When** the speaker requests it again, **Then** the response shows the
   weather and a `internal` (cache HIT) source indicator, with no external
   HTTPS request made.
3. **Given** the bypass city (São Paulo) and any cache state, **When** the
   speaker requests its weather, **Then** the response always shows a
   `external` (cache MISS) source indicator and triggers an external HTTPS
   request, every single time.

---

### User Story 2 — Operator generates demo traffic to make the leak visible (Priority: P1)

While Hubble (or any eBPF tracer) is running on the cluster, the operator
launches a load generator that fires a 90/10 mix: 90 % of requests go to
non-bypass cities and 10 % go to São Paulo. After ~30 seconds the operator
filters Hubble by the app's Pod and observes that only the São Paulo branch
produces packets to `api.open-meteo.com`, while all other traffic stays
inside the cluster (toward the Redis Pod's IP).

**Why this priority**: This is the second beat of the talk and the moment
where eBPF earns its place on stage. The story only works if the load
generator is reproducible, easy to launch, and produces a clean signal in
the kernel-level capture.

**Independent Test**: With the app deployed, run the load generator for
30 seconds. Aggregate Hubble flows by destination: non-bypass requests MUST
generate zero flows to the public internet, and the only public-internet
flows visible MUST originate from the São Paulo handler.

**Acceptance Scenarios**:

1. **Given** the app is deployed and the load generator runs at 90/10 split,
   **When** Hubble flows are aggregated by destination IP, **Then** the only
   external destination observed for the app Pod is the Open-Meteo API and
   it correlates 1-to-1 with São Paulo requests.
2. **Given** the load generator is stopped, **When** the operator inspects
   Hubble, **Then** outbound external traffic from the app Pod returns to
   zero within 60 seconds (no background egress).

---

### User Story 3 — Operator resets demo state mid-talk (Priority: P2)

If the demo gets out of sync (e.g. someone consulted São Paulo and the
audience missed it, or the speaker wants to re-show the cold-cache moment),
the operator can wipe the cache and re-run scenario 1 in under one minute,
without restarting the application Pod.

**Why this priority**: A talk is a live performance; a 30-second recovery
mechanism is the difference between a smooth demo and a panic moment. It is
P2 because P1 stories are still valuable without it, but in practice every
real-world talk uses this at least once.

**Independent Test**: With the app running and several cities cached,
trigger the cache reset (restart the Redis Pod or call a documented reset
mechanism). Within 60 seconds, every non-bypass city MUST again return
`external` (cache MISS) on its next request.

**Acceptance Scenarios**:

1. **Given** several non-bypass cities are cached, **When** the operator
   resets the cache, **Then** the next request to each of those cities
   returns `external` (cache MISS) again.
2. **Given** a cache reset just happened, **When** the operator queries the
   bypass city, **Then** behavior is unchanged (still `external`).

---

### User Story 4 — Audience member self-validates from the back of the room (Priority: P3)

A curious audience member on the same network opens the dashboard URL on
their laptop, picks any city, and sees the same `cache HIT/MISS` indicator
the speaker is showing. They cannot harm the demo state; the only possible
side-effect is one cache miss for an uncached city.

**Why this priority**: This is a "nice to have" — being reachable on the LAN
via MetalLB elevates the demo from screencast-grade to live-rig-grade — but
the talk's core message survives without it.

**Independent Test**: From a second machine on the same LAN as the kind
host, open the LoadBalancer IP in a browser and consult any city; the
result MUST be displayed with the source badge.

**Acceptance Scenarios**:

1. **Given** the cluster is reachable via MetalLB on the LAN, **When** a
   second device requests the dashboard URL, **Then** the dashboard renders
   and a city query returns a result with a source badge.

---

### Edge Cases

- **External API unreachable**: The app MUST surface a clear, non-stack-trace
  error in the UI ("could not reach the weather provider") and MUST NOT
  poison the cache with the error. A subsequent retry against a non-bypass
  city MUST attempt the external API again.
- **Redis unreachable**: The app MUST still serve non-bypass cities by going
  to the external API every time (degrading gracefully into "all-bypass"
  mode), and the source badge MUST clearly indicate `external` so the
  audience is not misled. A health endpoint SHOULD report the cache outage.
- **Stale clock**: If Redis serves an entry whose written-at timestamp is
  older than the configured TTL (e.g. clock skew between Pods), the entry
  MUST be treated as expired and refreshed from origin.
- **Unknown city in URL**: A request for a city not in the configured list
  MUST return a 4xx and MUST NOT trigger any external HTTPS request, to
  avoid the audience seeing "phantom" leaks.
- **Concurrent first request to same city**: Two simultaneous first requests
  for the same non-bypass city MAY both hit the external API; this is
  acceptable for a demo but MUST NOT corrupt the cache.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST expose a single web UI page that lets a user pick
  one city from a configured list and request its current weather.
- **FR-002**: For every weather lookup, system MUST display in the UI both
  the weather data (at minimum: temperature in °C and a short condition
  description) and a clearly visible source indicator. The indicator MUST
  show one of three states, distinguishable by both text and color:
  - `cache HIT (internal)` — non-bypass city served from Redis.
  - `cache MISS (external)` — non-bypass city served from the external
    provider (cold cache, expired entry, or Redis unreachable).
  - `BYPASS (always external)` — city listed in the bypass policy; never
    cached.
- **FR-003**: System MUST also expose the same lookup as a JSON endpoint
  whose response body includes the fields `city`, `display_name`,
  `temperature_c`, `condition`, `weather_code`, `source` (one of `"cache"`,
  `"origin"`, or `"bypass"`), and `fetched_at` (ISO-8601 timestamp). The
  same response MUST also set the HTTP header `X-Source` mirroring the
  `source` field, so the value is visible to clients that pipe through
  tools like `curl | grep` during a live demo.
- **FR-004**: System MUST honor a configurable list of "bypass cities".
  Requests for any bypass city MUST always go to the external weather
  provider and MUST NOT read from or write to the cache.
- **FR-005**: For non-bypass cities, system MUST return a cached result if
  one exists within the configured Time-To-Live (default: 1 hour); otherwise
  system MUST fetch from the external provider, store the result with the
  TTL, and return it.
- **FR-006**: System MUST emit one structured log line per HTTP request
  containing at minimum: timestamp, requested city, source decision
  (`cache` / `origin` / `bypass`), HTTP status, total latency in
  milliseconds, and a per-request correlation ID (`request_id`).
- **FR-007**: System MUST expose a liveness endpoint that returns 200 when
  the process is up, regardless of cache or external-provider availability.
- **FR-008**: System MUST NOT make any outbound network call other than
  (a) Redis (intra-cluster) for non-bypass cache reads/writes and (b) the
  configured external weather provider for cache misses and bypass cities.
  No telemetry, update checks, or pre-warming.
- **FR-009**: System MUST be deployable to a Kubernetes cluster as a single
  workload exposing a `LoadBalancer` Service, alongside a Redis workload
  in the same namespace, with all configuration provided via plain
  ConfigMap entries and environment variables (no Secrets).
- **FR-010**: System MUST ship with a load-generator script that produces a
  configurable mix of requests across cities (default: 90 % non-bypass /
  10 % bypass) for a configurable duration.
- **FR-011**: System MUST treat a request for a city outside the configured
  list as a client error (4xx) without making any external call.

### Key Entities *(include if feature involves data)*

- **City**: One of the cities the demo exposes; identified by a slug
  (e.g. `bogota`, `sao-paulo`). Carries human-readable name and the
  geographic coordinates needed for the weather lookup. Static, defined
  in configuration; not user-editable at runtime.
- **WeatherSnapshot**: The result of a weather lookup at a point in time.
  Belongs to one City. Includes temperature, condition, the timestamp
  when the snapshot was fetched from origin, and the source attribution
  (`cache` / `origin` / `bypass`). Cacheable for non-bypass cities up to
  the configured TTL.
- **BypassPolicy**: The configured set of cities that MUST always reach
  the external provider. Read at startup from configuration; not mutated
  at runtime.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Audience can read the source indicator (`internal cache` /
  `external API`) without leaning forward — i.e. the badge is visible from
  the back row at a typical conference-room projection.
- **SC-002**: After deployment to a clean cluster, the speaker can complete
  the first end-to-end demo (open UI, query each city, observe the source
  badge flip) in under 5 minutes.
- **SC-003**: With the load generator running at the default 90/10 split
  for one minute, the only external destination observed in kernel-level
  capture for the application's Pod is the weather provider, and 100 % of
  those flows correlate with bypass-city requests.
- **SC-004**: Non-bypass cities serve at least 95 % of requests from the
  internal cache once warmed, measured over a 5-minute load-generator run.
- **SC-005**: The operator can fully reset the demo to a cold-cache state
  in under 60 seconds without restarting the application workload.
- **SC-006**: A single user request returns to the browser in under
  2 seconds for cache hits and under 4 seconds for cache misses, on a
  laptop-grade host with a typical home/office uplink.
- **SC-007**: When the external provider is unreachable, the UI surfaces a
  human-readable error within 5 seconds; no stack traces, no blank page.

## Assumptions

- The kind cluster, MetalLB, and a working IP pool already exist on the
  speaker's Linux host; this project does not provision them.
- The speaker has Docker available locally to build the image and uses
  `kind load docker-image` to make it available to the cluster nodes.
- The external weather provider is publicly reachable and does not require
  an API key, account, or authentication for the volume of requests this
  demo will make.
- The talk audience is technical enough to read a small badge (`cache HIT`
  vs `cache MISS`); no localization beyond Spanish/English is required.
- Persistence across Redis restarts is explicitly out of scope; restarting
  Redis is, in fact, a supported way to reset demo state.
- HTTPS termination, TLS certificates, and authentication for the UI are
  out of scope — the demo runs on a private LAN during the talk.
- Observability beyond stdout structured logs and an optional Prometheus
  text endpoint is out of scope; eBPF/Hubble (provided by the cluster
  operator, not by this app) is the primary observability story.

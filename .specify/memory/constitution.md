<!--
SYNC IMPACT REPORT
==================
Version change: 1.0.0 → 1.0.1 (PATCH — clarification)
Modified principles:
  - III. Observable by Default — `source` enum extended from {"cache","origin"} to {"cache","origin","bypass"} to match the design that has been internally consistent across plan/data-model/contracts since /speckit-plan ran. No semantic change to the principle's intent.
Added sections: none
Removed sections: none
Templates requiring updates:
  - ✅ .specify/memory/constitution.md (this file)
  - ✅ specs/001-weather-cache-demo/spec.md — FR-002 and FR-003 wording reconciled with the 3-value enum.
  - n/a .specify/templates/* (no structural impact)
Follow-up TODOs: none

Previous report (v1.0.0):
  Initial ratification — 4 principles, demo & deployment constraints, governance.
-->

# weather-app Constitution

## Core Principles

### I. Single Binary, Single Process (NON-NEGOTIABLE)

The application MUST ship as ONE executable that serves UI, API, and cache logic
in the same process. There is no separate frontend or backend service. HTML is
rendered server-side from the same binary that talks to Redis and to the
external weather API.

Rationale: This demo is consumed visually during a live talk; reducing the
deployable unit to a single Pod makes the eBPF traffic story unambiguous —
any packet leaving the Pod is the app's own egress, never a sidecar's or a
front-end build's.

### II. Demo-Friendly Reproducibility (NON-NEGOTIABLE)

The project MUST be deployable on a pre-existing kind cluster with a single
load command for the image plus a single `kubectl apply` for the manifests.
The application MUST NOT require API keys, paid accounts, or per-environment
secrets. Configuration MUST be expressible via plain environment variables
with sane defaults baked into the manifests.

Rationale: The talk is delivered in front of an audience; failures unrelated
to eBPF (missing keys, broken state, flaky external auth) destroy the
narrative. Anything that cannot be re-run from a clean cluster in under
two minutes is out of scope.

### III. Observable by Default

Every HTTP response that resolves a city's weather MUST expose, in both the
UI and the JSON payload, the field `source` with one of these three values:

- `"cache"` — the response came from Redis (intra-cluster only).
- `"origin"` — the response was a cache miss for a non-bypass city, or a
  degraded path where Redis was unreachable; the request reached the
  external weather provider.
- `"bypass"` — the response was for a city configured in `BYPASS_CITIES`
  and was served from the external provider WITHOUT touching the cache.

The application MUST emit structured logs to stdout (JSON lines, one event
per request) including at minimum: `timestamp`, `city`, `source`,
`latency_ms`, `status_code`, `request_id`. The application SHOULD expose
`/healthz` (liveness) and `/metrics` (Prometheus text format) endpoints.

Rationale: The audience must be able to correlate what they see in the UI
with what eBPF/Hubble shows on the wire. Hiding the cache decision would
force the speaker to "trust me, that one went external" — unacceptable.
The three-value enum is what allows the loadgen-Hubble cross-check at
talk time: every `bypass` event in the metrics maps 1-to-1 to an external
flow, and `origin` events are bounded by the per-city TTL window.

### IV. eBPF Narrative Integrity (NON-NEGOTIABLE)

External egress MUST be 100% attributable to the deliberate cache-bypass
configuration. Concretely:

- Cities listed in `BYPASS_CITIES` (default: `sao-paulo`) MUST always go to
  the external API and MUST NOT read from or write to Redis.
- All other configured cities MUST be served from Redis when a fresh entry
  exists (TTL default: 1 hour) and MUST refresh from the external API
  exactly once per TTL window per city.
- The application MUST NOT perform background warm-ups, prefetches, retries
  on cache hits, telemetry beacons, or any other outbound HTTPS call beyond
  what the two rules above describe.

Rationale: The whole point of the demo is "the only thing leaking traffic
is the city we configured to leak". Any additional egress (telemetry,
update checks, analytics) breaks the story Hubble is supposed to tell.

## Demo & Deployment Constraints

- **Stack**: Go (single static binary, distroless or `alpine` image) +
  `html/template` for UI + `github.com/redis/go-redis/v9` for cache.
- **External API**: Open-Meteo (`api.open-meteo.com`) — no API key required.
- **Cache**: Redis 7, deployed as a `Deployment` (no persistence, no PVC).
  Restarting the Redis Pod MUST be a supported way to reset demo state.
- **Cluster**: pre-existing kind cluster on the speaker's Linux host.
  MetalLB is available; the app's `Service` SHOULD be `type: LoadBalancer`.
- **Image distribution**: `kind load docker-image` (no external registry).
- **Namespace**: `weather-demo`.
- **Cities (default)**: `bogota`, `lima`, `santiago`, `buenos-aires`,
  `caracas`, `quito`, `montevideo`, `sao-paulo` (last one bypassed).
- **No secrets**: any value the manifests need MUST be a plain ConfigMap
  entry or env var. If a future change introduces a secret, that change
  is a constitutional violation and requires an amendment.

## Development Workflow

- Source layout follows idiomatic Go: `cmd/weather-app/` for the entrypoint,
  `internal/` for non-exported packages (handlers, weather client, cache,
  config), `web/` for templates and static assets, `manifests/` for
  Kubernetes YAML, `loadgen/` for the demo traffic generator.
- The repository MUST contain a `Makefile` (or equivalent) with at least:
  `make build`, `make image`, `make load` (kind load), `make deploy`,
  `make undeploy`, `make loadgen`.
- Tests are pragmatic, not dogmatic: unit tests for the cache-decision
  logic and the Open-Meteo client are REQUIRED; UI tests are NOT required.
- Code review on PRs MUST verify each of the four Core Principles.
  Pre-commit checks SHOULD run `go vet` and `gofmt`.
- The default branch is `main`. Demo-day tags use `vX.Y.Z` semver.

## Governance

This Constitution supersedes ad-hoc decisions made during implementation.
Amendments require:

1. A PR that updates this document AND the Sync Impact Report at the top.
2. A version bump following SemVer: MAJOR for removing/redefining a
   non-negotiable principle, MINOR for adding a new principle or section,
   PATCH for clarifications and typo fixes.
3. Propagation to dependent templates under `.specify/templates/` and to
   any runtime guidance docs (e.g. `README.md`).

All `/speckit-plan` runs MUST execute the Constitution Check gate against
the four Core Principles above and refuse to proceed if any check fails
without a documented and accepted complexity justification.

**Version**: 1.0.1 | **Ratified**: 2026-05-14 | **Last Amended**: 2026-05-14

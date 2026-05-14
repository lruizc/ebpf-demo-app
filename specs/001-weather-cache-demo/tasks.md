---
description: "Task list for Weather Cache Demo (eBPF talk)"
---

# Tasks: Weather Cache Demo for eBPF Talk

**Input**: Design documents from `/specs/001-weather-cache-demo/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: Selectively included. Per Constitution v1.0.0 § Development Workflow, tests are required only for the **cache-decision pure function** (`internal/server/decide.go`) and the **Open-Meteo client parser** (`internal/weather/client.go`). UI/integration tests are explicitly **out of scope** (the live demo is the integration test).

**Organization**: Tasks are grouped by user story. P1 stories form the MVP; the demo is a viable talk asset after Phase 4 completes.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks).
- **[Story]**: Which user story this task belongs to (US1, US2, US3, US4).
- File paths are repository-relative (root: `/Users/luisruiz/workspace/talks/ebpf/weather-app/`).

## Path Conventions

Single Go project, `cmd/<binary>` + `internal/<package>` layout (see `plan.md` § Project Structure):

- App code: `cmd/weather-app/`, `internal/<pkg>/`
- UI assets: `web/templates/`, `web/static/`
- Deploy: `manifests/`, `loadgen/`
- Build: `Dockerfile`, `Makefile`, `go.mod`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project skeleton, Go module, baseline tooling.

- [ ] T001 Create directory layout: `cmd/weather-app/`, `internal/{config,weather,cache,server,obs}/`, `web/{templates,static}/`, `manifests/`, `loadgen/`
- [ ] T002 Initialize Go module: `go mod init github.com/luisruiz/weather-app` (Go 1.22+) at repo root, producing `go.mod`
- [ ] T003 [P] Add `.dockerignore` at repo root excluding `specs/`, `.cursor/`, `.specify/`, `.git/`, `*.md`, `tests/`, `coverage.out`
- [ ] T004 [P] Extend repo-root `.gitignore` with Go/build artifacts (`/weather-app`, `coverage.out`, `*.test`, `web/static/.cache/`) — verify the existing entries from constitution-init are preserved
- [ ] T005 [P] Create skeleton `README.md` at repo root with one-paragraph project description and a placeholder "Quickstart" section pointing to `specs/001-weather-cache-demo/quickstart.md`
- [ ] T006 Add Go dependencies to `go.mod` and `go.sum`: `github.com/redis/go-redis/v9`, `github.com/prometheus/client_golang`, plus test deps `github.com/alicebob/miniredis/v2`. Run `go mod tidy` after declaring them.

**Checkpoint**: `go build ./...` exits 0 (no code yet, but module resolves).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Packages that every user story depends on (config, logging, weather client, cache wrapper). No HTTP routing yet.

**⚠️ CRITICAL**: No user story work can begin until Phase 2 is complete.

- [ ] T007 [P] Implement default city catalog and `City` struct in `internal/config/cities.go`: 8 cities per `research.md` § R-003, with `Slug`, `DisplayName`, `Latitude`, `Longitude`; expose `func DefaultCities() []City`
- [ ] T008 [P] Implement env-var configuration loader in `internal/config/config.go`: type `Config` with all fields from `data-model.md` § Configuration entity; `func Load() (*Config, error)`; validate slug regex, lat/lon ranges, and that every entry of `BypassCities` exists in `Cities`; defaults baked in
- [ ] T009 [P] Implement structured logger setup in `internal/obs/log.go`: `func NewLogger(level string) *slog.Logger` returning a JSON handler bound to `os.Stdout`; helper `func RequestID() string` returning a 16-hex-char random ID
- [ ] T010 [P] Implement Open-Meteo HTTP client in `internal/weather/client.go`: type `Client` with constructor `New(baseURL string, httpClient *http.Client) *Client`; method `Fetch(ctx context.Context, city config.City) (WeatherSnapshot, error)`; uses 3 s timeout (per `contracts/http-api.md`); maps `weathercode` → condition string via private `codeToCondition` table; ALWAYS sets `FetchedAt = time.Now().UTC()` from Open-Meteo's `time` field if present, falls back to `time.Now().UTC()`
- [ ] T011 [P] Unit tests for the Open-Meteo client in `internal/weather/client_test.go` using `httptest.NewServer` for the upstream: cover happy path, 5xx response (returns error), malformed JSON, timeout, and at least three different `weathercode` values mapping to expected condition strings — REQUIRED by Constitution Development Workflow
- [ ] T012 [P] Implement Redis cache wrapper in `internal/cache/redis.go`: type `Store` with constructor `New(addr string) (*Store, error)`; methods `Get(ctx, slug) (WeatherSnapshot, bool, error)` (returns `(_, false, nil)` on miss or unmarshal-corrupt) and `SetWithTTL(ctx, snap, ttl)`; key prefix `weather:v1:`; 300 ms operation deadline
- [ ] T013 [P] Unit tests for the Redis cache in `internal/cache/redis_test.go` using `miniredis.RunT(t)`: cover hit, miss, TTL expiry advance via `mr.FastForward`, and corrupt-JSON treated as miss — REQUIRED by Constitution Development Workflow
- [ ] T014 Define the shared `Snapshot` type (idiomatic Go: avoid stutter — `weather.Snapshot`, not `weather.WeatherSnapshot`) in `internal/weather/snapshot.go` with JSON tags exactly matching `contracts/http-api.md` (`city`, `display_name`, `temperature_c`, `condition`, `weather_code`, `fetched_at`); the `source` field is set by handlers, not stored — keep it as a runtime-only field with `json:"source,omitempty"` and document it.

**Checkpoint**: `go test ./internal/...` is green; no HTTP server exists yet.

---

## Phase 3: User Story 1 — Speaker demonstrates cache vs origin in UI (Priority: P1) 🎯 MVP

**Goal**: A working dashboard at the LB IP where the speaker can pick any city, click "Consultar", and see a giant `cache HIT (internal)` / `cache MISS (external)` / `BYPASS (always external)` badge — running on the existing kind cluster, deployed via `kubectl apply -k manifests/`.

**Independent Test**: With the app deployed, open `http://<LB-IP>/`, query Bogotá twice (badge flips green), then query São Paulo three times (badge stays red), then query an unknown city (UI shows a clean error, no stack trace).

### Implementation

- [ ] T015 [P] [US1] Implement the cache-vs-origin pure decision function in `internal/server/decide.go`: `func Decide(cfg *config.Config, slug string, cached *weather.Snapshot, now time.Time) Decision` returning a struct with `{Action: bypass|origin|cache, Reason: string}`; ZERO I/O, fully deterministic
- [ ] T016 [P] [US1] Unit tests for the decision function in `internal/server/decide_test.go` covering all four rows of the side-effects table in `contracts/http-api.md` (unknown city, bypass city, non-bypass cache hit, non-bypass cache miss) plus expired-cache fallback — REQUIRED by Constitution Development Workflow
- [ ] T017 [P] [US1] Implement Prometheus collectors in `internal/server/metrics.go`: register the four custom metrics from `contracts/ops.md` (`weather_requests_total`, `weather_request_duration_seconds`, `weather_origin_requests_total`, `weather_cache_operations_total`); expose `Handler() http.Handler` returning `promhttp.HandlerFor`
- [ ] T018 [P] [US1] Author `web/templates/index.html` per `contracts/ui.md`: `<title>`, heading, subtitle, `<select>` populated from `.Cities`, "Consultar" `<button>`, empty result panel, hidden error region, optional footer with `.PodName` and `.AppVersion`
- [ ] T019 [P] [US1] Author `web/static/styles.css` per `contracts/ui.md`: `.badge--internal` (green), `.badge--external` (orange), `.badge--bypass` (red); badge font-size ≥ 28 px; layout readable on a projector
- [ ] T020 [P] [US1] Author `web/static/app.js` (≤ 50 lines, vanilla JS): on button click, GET `/api/weather?city=<slug>`, on 2xx render result + badge, on 4xx/5xx show error message verbatim from JSON envelope (no stack traces). Uses `X-Source` response header as a sanity check.
- [ ] T021 [US1] Implement HTTP handlers in `internal/server/handlers.go`: `IndexHandler` (renders template with embedded `web/`), `WeatherHandler` (calls `Decide` then orchestrates cache + origin per the pseudocode in `contracts/http-api.md`), `HealthHandler` (always 200 `ok\n`); every response sets `X-Request-ID`; the JSON handler sets `X-Source` mirroring `source`. Records all four metrics from T017. Logs one structured line per request via `slog`.
- [ ] T022 [US1] Implement static assets and routing in `internal/server/routes.go`: a single `*http.ServeMux` registering `GET /`, `GET /api/weather`, `GET /healthz`, `GET /metrics`, and `GET /static/` (served via `http.FileServer` over the `go:embed` filesystem from `web/static`). Method-mismatched requests return 405 with `Allow: GET`.
- [ ] T023 [US1] Wire everything in `cmd/weather-app/main.go`: load config, build logger, dial Redis (with one bounded retry then continue degraded — log a warning, don't crash), instantiate handlers, start `http.Server` with `ReadHeaderTimeout: 5*time.Second`, `WriteTimeout: 10*time.Second`; handle SIGTERM with a 10 s graceful shutdown
- [ ] T024 [US1] Author `Dockerfile` at repo root per `research.md` § R-009: declare `ARG VERSION=dev` near the top, stage 1 `golang:1.22-bookworm` runs `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/weather-app ./cmd/weather-app`; stage 2 `gcr.io/distroless/static-debian12:nonroot` with the binary at `/weather-app` and `web/` at `/web/`. ENTRYPOINT `["/weather-app"]`. EXPOSE 8080. **Do NOT shell out to `git describe` inside the build** — `.dockerignore` excludes `.git/` and the build context wouldn't have it anyway. Version is supplied as a build-arg by the Makefile.
- [ ] T025 [US1] Author `Makefile` at repo root with US1-related targets: set `KIND_CLUSTER ?= kind` and `VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)`; targets `build`, `test`, `image` (= `docker build --build-arg VERSION=$(VERSION) -t weather-app:dev .`), `load` (= `kind load docker-image weather-app:dev --name $(KIND_CLUSTER)`); include a top-of-file `.PHONY` declaration; `help` target listing all targets
- [ ] T026 [P] [US1] Author `manifests/namespace.yaml`: `Namespace` `weather-demo` with label `app.kubernetes.io/part-of: weather-demo`
- [ ] T027 [P] [US1] Author `manifests/configmap.yaml`: ConfigMap `weather-app-config` in `weather-demo` with keys `CITIES` (JSON array of the default 8), `BYPASS_CITIES` (`sao-paulo`), `TTL_SECONDS` (`3600`), `REDIS_ADDR` (`redis:6379`), `OPEN_METEO_URL` (`https://api.open-meteo.com/v1/forecast`), `LISTEN_ADDR` (`:8080`), `LOG_LEVEL` (`info`)
- [ ] T028 [P] [US1] Author `manifests/redis-deployment.yaml`: `Deployment` named `redis`, 1 replica, image `redis:7-alpine`, container port `6379`, `securityContext: {runAsNonRoot: true, runAsUser: 999}`, no PVC; livenessProbe `redis-cli ping`; resource requests `100m/64Mi`, limits `200m/128Mi`
- [ ] T029 [P] [US1] Author `manifests/redis-service.yaml`: `Service` named `redis`, ClusterIP, port 6379 → 6379, selector `app=redis`
- [ ] T030 [P] [US1] Author `manifests/weather-app-deployment.yaml`: `Deployment` named `weather-app`, 1 replica, image `weather-app:dev`, `imagePullPolicy: IfNotPresent`, port `8080`; envFrom the ConfigMap; livenessProbe + readinessProbe both on `GET /healthz` (initialDelay 2s, period 10s); resource requests `50m/32Mi`, limits `200m/128Mi`; `securityContext.runAsNonRoot: true`
- [ ] T031 [P] [US1] Author `manifests/weather-app-service.yaml`: `Service` named `weather-app`, `type: LoadBalancer`, port 80 → 8080, selector `app=weather-app` — relies on the existing MetalLB IP pool
- [ ] T032 [US1] Author `manifests/kustomization.yaml` listing the six manifests above as `resources` and setting `namespace: weather-demo`
- [ ] T033 [US1] Extend `Makefile` with deploy targets: `deploy` (= `kubectl apply -k manifests/`), `undeploy` (= `kubectl delete -k manifests/`), `wait` (= `kubectl -n weather-demo wait deploy/weather-app deploy/redis --for condition=available --timeout=120s`), `lb-ip` (one-liner that prints the LoadBalancer IP)

**Checkpoint**: From a clean cluster, `make image && make load && make deploy && make wait` brings the app up; `curl http://$(make -s lb-ip)/api/weather?city=bogota` returns JSON with `"source":"origin"` first, `"source":"cache"` next; `?city=sao-paulo` always returns `"source":"bypass"`. **MVP achieved.**

---

## Phase 4: User Story 2 — Operator generates demo traffic for the eBPF moment (Priority: P1)

**Goal**: A reproducible 90/10 traffic split (non-bypass / São Paulo) that makes the eBPF/Hubble visualization show exactly one external destination correlated 1-to-1 with São Paulo requests.

**Independent Test**: With US1 deployed, run the loadgen for 30 seconds; in Hubble, filter by the app Pod and confirm: (a) zero flows to public internet from non-bypass requests, (b) the only external destination observed is `api.open-meteo.com` and corresponds to São Paulo traffic.

### Implementation

- [ ] T034 [P] [US2] Author `loadgen/loadgen.sh`: bash + `curl`, configurable via env vars `URL` (required), `RATE` (default `30` rps), `DURATION` (default `60` s), `BYPASS_RATIO` (default `10` percent); uses the catalog of cities from a hard-coded array (mirrors the default catalog in `internal/config/cities.go`); prints a one-line summary every 10 s with `cache_hit / cache_miss / bypass / error` counts parsed from the `X-Source` header
- [ ] T035 [P] [US2] Author `loadgen/loadgen.yaml`: a Kubernetes `Job` in `weather-demo` namespace using the `curlimages/curl` image; mounts `loadgen.sh` from a ConfigMap and runs it with `/bin/sh`; `URL` defaults to `http://weather-app.weather-demo.svc.cluster.local`; `restartPolicy: Never`, `backoffLimit: 0`. Includes the inline ConfigMap `loadgen-script` in the same file or as a sibling
- [ ] T036 [US2] Extend `Makefile` with `loadgen` target: `URL ?= http://$(shell $(MAKE) -s lb-ip)`; runs `loadgen/loadgen.sh` against `$(URL)`. Add `loadgen-job` target that applies `loadgen/loadgen.yaml` and follows logs (`kubectl -n weather-demo logs -f job/loadgen`)
- [ ] T037 [US2] Add a "Demo runbook" section to `README.md` describing the exact sequence to run during the talk (open dashboard → run loadgen → show Hubble). Cross-reference `quickstart.md`. Note the expected metric assertions: `weather_origin_requests_total{city="sao-paulo"}` grows monotonically; everything else has at most one increment per TTL window

**Checkpoint**: `make loadgen` runs cleanly for 60 s and prints the per-source counts; running `kubectl logs -f job/loadgen` in another window mirrors the same output. The talk's "eBPF moment" is now reproducible.

---

## Phase 5: User Story 3 — Operator resets demo state mid-talk (Priority: P2)

**Goal**: A one-command, sub-60 s reset that wipes the cache without touching the app Pod, so the speaker can re-show the cold-cache moment if the demo gets out of sync.

**Independent Test**: Cache several non-bypass cities, run `make reset`, wait until Redis is `Ready` again, query each cached city; every one MUST return `"source":"origin"` again. Bypass-city behavior MUST be unchanged.

### Implementation

- [ ] T038 [US3] Extend `Makefile` with `reset` target: `kubectl -n weather-demo rollout restart deployment/redis && kubectl -n weather-demo rollout status deployment/redis --timeout=60s`
- [ ] T039 [US3] Add a "Reset during the talk" subsection to `README.md` documenting the procedure, the expected ~5 s downtime, and the explicit guarantee that the `weather-app` Pod is NOT restarted (so request counters and goroutine lifetime stay continuous — useful when correlating with eBPF capture)

**Checkpoint**: `make reset` exits 0 in <30 s and the next non-bypass query returns `origin` again.

---

## Phase 6: User Story 4 — Audience self-validates from the LAN (Priority: P3)

**Goal**: A second device on the same network can hit the dashboard URL and see the same `HIT/MISS` indicator the speaker sees on the projector.

**Independent Test**: From a second machine on the LAN, open `http://<MetalLB-IP>/` in a browser and consult any city; the dashboard renders and a result is returned with the source badge.

### Implementation

- [ ] T040 [US4] Document in `README.md` how the audience can reach the demo (one paragraph + the `make lb-ip` recipe). Include a short note that there is no auth, no rate limit, and that an audience cache miss costs the demo at most one extra origin call (acceptable per spec edge cases)

**Checkpoint**: Curling the LB IP from a non-cluster machine on the LAN returns a 200 with valid JSON.

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Last-mile quality gates and documentation that touch multiple stories.

- [ ] T041 [P] Final `README.md` pass: project description, architecture diagram (ASCII or mermaid), constitution highlights, link to spec/plan/quickstart, list of make targets, troubleshooting cheat-sheet from `quickstart.md` § Troubleshooting
- [ ] T042 [P] Author `LICENSE` (MIT) at repo root
- [ ] T043 [P] Run `gofmt -w .` and `go vet ./...` across the repo; fix any findings
- [ ] T044 [P] Run `go test -race ./...` and confirm 100 % pass rate on the targeted-test files (`decide_test.go`, `client_test.go`, `redis_test.go`)
- [ ] T045 Verify the `quickstart.md` end-to-end on a clean `weather-demo` namespace: `make image → load → deploy → wait → curl /api/weather → make loadgen → make reset → make undeploy`. Update the doc if any command needs adjustment.
- [ ] T046 Constitutional compliance review: confirm zero `Secret` resources in `manifests/`, zero outbound dial-out from app code other than Redis and Open-Meteo (grep for `http.Client` / `Get(` instances), and that `source` is unconditionally set on every JSON response. Document the review outcome in a short comment block at the top of `cmd/weather-app/main.go`.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: no dependencies — can start immediately.
- **Phase 2 (Foundational)**: depends on Phase 1 — BLOCKS all user stories.
- **Phase 3 (US1 — MVP)**: depends on Phase 2.
- **Phase 4 (US2)**: depends on Phase 3 (needs the deployed app to generate traffic against).
- **Phase 5 (US3)**: depends on Phase 3 (needs `manifests/redis-deployment.yaml` to exist for `rollout restart`).
- **Phase 6 (US4)**: depends on Phase 3 (needs the LoadBalancer Service to exist).
- **Phase 7 (Polish)**: depends on whatever stories are in scope; ideally all four.

### Within Each Story

- Foundational packages (Phase 2) before any handler work.
- Handlers (T021/T022) depend on T015–T017 (decision, metrics, snapshot type) and T012 (cache).
- Container image (T024) depends on a builds-clean Go module (Phase 2 + T021–T023 done).
- Manifests don't depend on the image existing for `kubectl apply` to succeed, but the workload won't become Ready until T024 + `make load` have run.

### Parallel Opportunities

- **Phase 1**: T003, T004, T005 are all independent files → can run together.
- **Phase 2**: T007, T008, T009, T010, T012 are independent packages → all can run together. T011 follows T010, T013 follows T012, T014 is independent.
- **Phase 3 — US1**: T015, T017, T018, T019, T020 touch different files and can be done in parallel. T026–T031 (manifests) are six independent YAML files. T021–T023 are sequential (handlers → routes → main).
- **Phase 4 — US2**: T034 and T035 are independent files; T036 and T037 follow.
- **Phase 7**: T041, T042, T043, T044 are all parallelizable.

---

## Parallel Example — Phase 2 (Foundational)

```text
# All five of these can be implemented concurrently; none import each other:
Task: T007 [P] Default city catalog in internal/config/cities.go
Task: T008 [P] Env-var Config loader in internal/config/config.go
Task: T009 [P] slog JSON logger in internal/obs/log.go
Task: T010 [P] Open-Meteo client in internal/weather/client.go
Task: T012 [P] Redis cache wrapper in internal/cache/redis.go

# Tests depend on their respective implementations:
Task (after T010): T011 [P] [Tests] internal/weather/client_test.go
Task (after T012): T013 [P] [Tests] internal/cache/redis_test.go
```

## Parallel Example — Phase 3 (US1) UI assets

```text
# Three independent web assets — easy to split among contributors:
Task: T018 [P] [US1] web/templates/index.html
Task: T019 [P] [US1] web/static/styles.css
Task: T020 [P] [US1] web/static/app.js
```

## Parallel Example — Phase 3 (US1) manifests

```text
# Six independent manifests — generate as a batch then assemble kustomization.yaml in T032:
Task: T026 [P] [US1] manifests/namespace.yaml
Task: T027 [P] [US1] manifests/configmap.yaml
Task: T028 [P] [US1] manifests/redis-deployment.yaml
Task: T029 [P] [US1] manifests/redis-service.yaml
Task: T030 [P] [US1] manifests/weather-app-deployment.yaml
Task: T031 [P] [US1] manifests/weather-app-service.yaml
```

---

## Implementation Strategy

### MVP First (Phases 1 → 2 → 3, that is User Story 1 only)

1. Phase 1 (Setup) — ~30 minutes.
2. Phase 2 (Foundational) — ~2 hours. Includes the two required test suites.
3. Phase 3 (US1) — ~3 hours. End state: app running on the kind cluster, dashboard reachable on the LAN, badge flips correctly.
4. **STOP and validate**: walk through US1's Independent Test on the actual cluster.

After this, the talk is technically deliverable: you can demo the cache decision visually, even before the eBPF segment exists. This is the safe rollback point.

### Incremental Delivery

- Add Phase 4 (US2) → loadgen reproducible → eBPF moment becomes scriptable.
- Add Phase 5 (US3) → reset workflow → recovery story for live demo.
- Add Phase 6 (US4) → LAN access → audience-friendliness.
- Phase 7 (Polish) at the end → README, license, consistency review.

### Solo Strategy (since this is a one-person demo project)

- Do Phases 1 and 2 sequentially (helps you load the architecture into your head).
- Inside Phase 3, batch the manifests (T026–T031) and the UI assets (T018–T020) — those are the highest-parallelism stretches.
- Run `go test -race ./...` after every commit to keep the foundational tests green.
- Commit at every checkpoint above; each one is a viable rollback.

---

## Notes

- `[P]` tasks = different files, no dependencies on incomplete tasks.
- `[Story]` label maps a task to its user story for traceability.
- Each user story phase ends in a checkpoint that should leave the demo in a runnable state.
- Two test suites are mandated by the constitution (T011, T013); a third (T016) is added because the cache-decision logic is the riskiest piece of the demo. No other tests are required.
- Avoid: vague tasks, edits that touch the same file across `[P]` siblings, or cross-story dependencies that break independence.
- If anything in Phase 3 is taking longer than estimated, consider deferring T037 + T040 to after the talk — the demo works without those documentation pieces.

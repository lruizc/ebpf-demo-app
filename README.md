# weather-app — eBPF talk demo

A single Go binary that serves a weather dashboard for South-American cities,
backed by a Redis cache. One city (`sao-paulo` by default) is configured to
**always bypass the cache**, producing a constant stream of external HTTPS traffic —
the perfect target to isolate with eBPF / Hubble during a live talk.

> Full specification, plan and quickstart: [`specs/001-weather-cache-demo/`](specs/001-weather-cache-demo/)

---

## Architecture

```
Browser / curl
     │  GET /api/weather?city=bogota
     ▼
┌─────────────────────────────────────┐
│  weather-app Pod  (single binary)   │
│                                     │
│  handler → Decide(cfg, slug, cache) │
│      │                              │
│   cache ──── Redis Pod ─────────────┼── ClusterIP (internal traffic only)
│   origin/bypass                     │
│      │                              │
│      └─── api.open-meteo.com ───────┼── EGRESS (only on miss or bypass)
└─────────────────────────────────────┘
         ▲
    MetalLB LoadBalancer IP   ← audience + speaker + loadgen
```

The `source` field in every JSON response (`cache` / `origin` / `bypass`) is the
signal that eBPF/Hubble confirms at the kernel level.

**Dashboard badges:**

| Badge | Color | eBPF meaning |
|-------|-------|--------------|
| `cache HIT (internal)` | 🟢 Green | Zero packets to the internet |
| `cache MISS (external)` | 🟠 Orange | One outbound flow to Open-Meteo |
| `BYPASS (always external)` | 🔴 Red | Outbound flow always visible |

---

## Where each command runs

> **All deployment commands run on the Linux tower** where the kind cluster is running.
> The Mac is only used for editing code.

| Command | Where |
|---|---|
| `make build` / `make test` | Any machine with Go installed |
| `make image` | Linux tower (requires Docker) |
| `make load` | Linux tower (requires `kind` CLI) |
| `make deploy` / `make wait` / `make lb-ip` | Linux tower (requires `kubectl`) |
| `make loadgen` / `make reset` / `make logs` | Linux tower |

## Quickstart (on the Linux tower)

Step-by-step guide: [`specs/001-weather-cache-demo/quickstart.md`](specs/001-weather-cache-demo/quickstart.md)

```bash
# 1. Clone the repo on the Linux tower
git clone lruizc.github.com:lruizc/ebpf-demo-app.git
cd ebpf-demo-app
git checkout 001-weather-cache-demo

# 2. Build the Docker image (tag: weather-app:dev)
make image

# 3. Load the image into kind
#    If your cluster has a different name than "ebpf-demo":
#    make load KIND_CLUSTER=<name>
make load

# 4. Deploy to Kubernetes
make deploy
make wait      # waits for Redis + weather-app to be Available

# 5. Get the IP and open the dashboard
make lb-ip     # e.g.: 192.168.64.100
# Open http://<IP>/ in the browser
```

**Quick verification:**
```bash
APP_IP=$(make -s lb-ip)
curl "http://${APP_IP}/api/weather?city=bogota"    # → "source":"origin"
curl "http://${APP_IP}/api/weather?city=bogota"    # → "source":"cache"
curl "http://${APP_IP}/api/weather?city=sao-paulo" # → "source":"bypass"
```

---

## Make targets

| Target | Description |
|---|---|
| `make build` | Compile the binary locally |
| `make test` | Run unit tests (with race detector) |
| `make image` | Build the Docker image (`weather-app:dev`) |
| `make load` | Load the image into kind |
| `make deploy` | `kubectl apply -k manifests/` |
| `make undeploy` | `kubectl delete -k manifests/` |
| `make wait` | Wait for both Deployments to be Available |
| `make lb-ip` | Print the MetalLB LoadBalancer IP |
| `make loadgen` | Run the 90/10 load generator locally |
| `make loadgen-job` | Run the load generator as a Job inside the cluster and tail logs |
| `make loadgen-clean` | Delete the loadgen Job and its ConfigMap |
| `make reset` | Restart Redis to wipe the cache |
| `make logs` | Tail JSON structured logs from the app |
| `make help` | Show all targets |

---

## Demo runbook (talk sequence)

> All commands run on the **Linux tower** from the repo directory.

### 1. Pre-talk — set up the cluster (on the Linux tower)

```bash
make image && make load && make deploy && make wait
APP_IP=$(make -s lb-ip)
echo "Dashboard: http://${APP_IP}/"
```

### 2. Show the dashboard on the projector

Open `http://<APP_IP>/` in the presenter's browser.

```bash
# Verify the three states:
curl -s "http://${APP_IP}/api/weather?city=bogota"    # source: origin (first time)
curl -s "http://${APP_IP}/api/weather?city=bogota"    # source: cache  (within TTL)
curl -s "http://${APP_IP}/api/weather?city=sao-paulo" # source: bypass (always)
```

### 3. Start the load generator (eBPF moment)

In a separate terminal — this produces the traffic that Hubble/Cilium observes:

```bash
# Option A: locally against the MetalLB IP
APP_IP=$(make -s lb-ip) make loadgen

# Option B: Job inside the cluster (recommended for the talk)
make loadgen-job
```

The generator produces ~90% cache hits (internal traffic) and ~10% bypass requests to
`api.open-meteo.com` (external traffic visible in eBPF).

### 4. Show Hubble / eBPF

With the load generator running, filter flows in another terminal:

```bash
# Hubble CLI (if installed)
hubble observe --pod weather-demo/weather-app --follow

# External flows only
hubble observe --pod weather-demo/weather-app \
  --verdict FORWARDED --follow \
  | grep -v "redis"
```

**Expected result:** The only external flows are to `api.open-meteo.com`
and they correlate 1:1 with `sao-paulo` requests.

### 5. Verify with Prometheus metrics

```bash
curl -s "http://${APP_IP}/metrics" \
  | grep weather_origin_requests_total
# weather_origin_requests_total{city="sao-paulo",outcome="success"} X  ← grows
# weather_origin_requests_total{city="bogota",outcome="success"}    1  ← only 1
```

---

## Reset mid-talk

If the demo gets out of sync or you want to show the cold-cache state again:

```bash
make reset
```

This runs `kubectl rollout restart deployment/redis`, which:

- **Wipes the entire cache** (the Redis pod is ephemeral, no PVC)
- **Does NOT restart the `weather-app` pod** — Prometheus counters and process
  lifecycle remain continuous (useful for correlating with eBPF captures that use the process PID)
- Redis downtime is ~3-5 seconds; during that window the app runs degraded (no cache) and keeps responding

---

## Audience access from the LAN

The MetalLB IP is reachable from any device on the same network:

```bash
make lb-ip    # e.g.: 192.168.64.100
```

From the audience's phone or laptop:

```
http://192.168.64.100/
```

No authentication, no rate limit. An audience cache miss generates at most one extra
call to Open-Meteo per city per hour — acceptable and even useful for enriching the
Hubble visualization.

---

## Configuration (env vars / ConfigMap)

| Key | Default | Description |
|---|---|---|
| `BYPASS_CITIES` | `sao-paulo` | Comma-separated city slugs that always go to origin |
| `TTL_SECONDS` | `3600` | Cache TTL in seconds (1 hour) |
| `REDIS_ADDR` | `redis:6379` | Redis host:port |
| `OPEN_METEO_URL` | `https://api.open-meteo.com/v1/forecast` | Weather API base URL |
| `LISTEN_ADDR` | `:8080` | HTTP bind address |
| `LOG_LEVEL` | `info` | Log level: debug/info/warn/error |
| `CITIES` | *(8 SA cities)* | JSON array override for the city catalog |

Edit `manifests/configmap.yaml` to change the config in Kubernetes.

---

## Troubleshooting

```bash
# App not responding
kubectl -n weather-demo get pods
kubectl -n weather-demo describe pod -l app=weather-app

# Redis not connecting
kubectl -n weather-demo logs -l app=redis
kubectl -n weather-demo exec -it deploy/redis -- redis-cli ping

# Empty MetalLB IP
kubectl -n weather-demo get svc weather-app   # check EXTERNAL-IP

# JSON structured logs
make logs | jq .
```

---

## Constitution (v1.0.1)

Non-negotiable principles: [`.specify/memory/constitution.md`](.specify/memory/constitution.md)

The only outbound TCP dials from the process are:
1. `redis:6379` — cache reads/writes
2. `api.open-meteo.com` — weather fetch (only on miss or bypass)

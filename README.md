# weather-app — eBPF talk demo

A single-binary Go web service that serves a weather dashboard for South-American
cities backed by a Redis cache. One city (`sao-paulo` by default) is configured
to **always bypass the cache**, producing a constant trickle of external HTTPS
traffic — the perfect target to isolate with eBPF / Hubble during a live talk.

> Full specification, plan, and quickstart: [`specs/001-weather-cache-demo/`](specs/001-weather-cache-demo/)

## Quickstart

See the detailed step-by-step guide: [`specs/001-weather-cache-demo/quickstart.md`](specs/001-weather-cache-demo/quickstart.md)

```bash
make image                    # docker build → weather-app:dev
make load                     # kind load docker-image weather-app:dev
kubectl apply -k manifests/   # deploy namespace + redis + app
make wait                     # wait for both deployments to be Ready
APP_IP=$(make -s lb-ip)
echo "open http://${APP_IP}/"
```

## Make targets

| Target | Description |
|---|---|
| `make build` | Compile the binary locally |
| `make test` | Run unit tests |
| `make image` | Build the Docker image (`weather-app:dev`) |
| `make load` | Load the image into kind |
| `make deploy` | `kubectl apply -k manifests/` |
| `make undeploy` | `kubectl delete -k manifests/` |
| `make wait` | Wait for deployments to be available |
| `make lb-ip` | Print the MetalLB LoadBalancer IP |
| `make loadgen` | Run the 90/10 load generator |
| `make reset` | Restart Redis to wipe the cache |
| `make help` | Show all targets |

## Architecture

```
Browser / curl
     │  GET /api/weather?city=bogota
     ▼
┌─────────────────────────────────┐
│  weather-app Pod                │
│  ┌──────────────────────────┐   │
│  │  handler: Decide()       │   │
│  │  ┌─────────┐ ┌────────┐  │   │
│  │  │  Redis  │ │Open-   │  │   │
│  │  │  cache  │ │Meteo   │  │   │────► api.open-meteo.com (bypass/miss only)
│  │  │  GET/SET│ │HTTP    │  │   │
│  │  └────┬────┘ └────────┘  │   │
│  │       │ intra-cluster     │   │
│  └───────┼──────────────────┘   │
└──────────┼──────────────────────┘
           │
    ┌──────▼──────┐
    │  Redis Pod  │  ← BYPASS cities never reach here
    └─────────────┘
```

The `source` field in every response (`cache` / `origin` / `bypass`) is the
signal that eBPF/Hubble confirms at the kernel level.

## Configuration (env vars / ConfigMap keys)

| Key | Default | Description |
|---|---|---|
| `BYPASS_CITIES` | `sao-paulo` | Comma-separated city slugs that always go to origin |
| `TTL_SECONDS` | `3600` | Cache TTL in seconds |
| `REDIS_ADDR` | `redis:6379` | Redis host:port |
| `OPEN_METEO_URL` | `https://api.open-meteo.com/v1/forecast` | Weather API base URL |
| `LISTEN_ADDR` | `:8080` | HTTP bind address |
| `LOG_LEVEL` | `info` | Log level: debug/info/warn/error |
| `CITIES` | *(8 SA cities)* | JSON array override for the city catalog |

## Constitution

Non-negotiable principles (v1.0.1): [`.specify/memory/constitution.md`](.specify/memory/constitution.md)

# Quickstart — weather-app demo on an existing kind cluster

**Feature**: Weather Cache Demo for eBPF Talk
**Audience**: the speaker, on talk day

This quickstart assumes:

- A kind cluster is already running. Default cluster name: `kind`. Override with `KIND_CLUSTER=<name>`.
- MetalLB is installed and has at least one IP in its address pool.
- Docker is available locally and has built the image (`make image` does it for you).
- The repo is at `/Users/luisruiz/workspace/talks/ebpf/weather-app` (macOS dev machine) **or** mirrored to the Linux tower where the kind cluster lives. The commands below run from the repo root.

---

## TL;DR — fastest path

```bash
make image                                                  # docker build → weather-app:dev
make load                                                   # kind load docker-image weather-app:dev
kubectl apply -k manifests/                                 # ns + redis + app + svc
kubectl -n weather-demo wait deploy/weather-app deploy/redis --for condition=available --timeout=120s
APP_IP=$(kubectl -n weather-demo get svc weather-app -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "open http://${APP_IP}/"
```

If everything is green, opening that URL shows the dashboard.

---

## 1. Build the container image

```bash
make image
```

This runs:

```bash
docker build -t weather-app:dev .
```

Multi-stage build: ~30 s on a warm cache, ~90 s cold. Final image is a
distroless static image (~15 MB).

## 2. Load the image into kind

```bash
make load
```

Equivalent to:

```bash
kind load docker-image weather-app:dev --name "${KIND_CLUSTER:-kind}"
```

If your cluster has a different name, run `KIND_CLUSTER=mycluster make load`.

## 3. Deploy the manifests

```bash
make deploy
```

Equivalent to:

```bash
kubectl apply -k manifests/
```

This creates, in order:

1. Namespace `weather-demo`.
2. ConfigMap `weather-app-config` (city catalog, bypass list, TTL, etc.).
3. Redis `Deployment` + `Service` (`redis:6379`, ClusterIP).
4. weather-app `Deployment` + `Service` (LoadBalancer, port 80 → 8080).

Wait for everything to be ready:

```bash
kubectl -n weather-demo wait deploy/weather-app deploy/redis \
  --for condition=available --timeout=120s
```

## 4. Find the app's URL

```bash
kubectl -n weather-demo get svc weather-app
```

Look for the `EXTERNAL-IP` column (assigned by MetalLB). In one liner:

```bash
APP_IP=$(kubectl -n weather-demo get svc weather-app -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "http://${APP_IP}/"
```

If `EXTERNAL-IP` is `<pending>` for more than ~30 s, MetalLB has no
allocatable IPs left or its speaker isn't running. Outside the scope of
this app, but here's a safety net: `kubectl port-forward -n weather-demo svc/weather-app 8080:80`.

## 5. Verify the demo behavior

### 5a. Cache miss → cache hit

```bash
curl -sS "http://${APP_IP}/api/weather?city=bogota" | jq .source
# → "origin"      (first call, cold cache)

curl -sS "http://${APP_IP}/api/weather?city=bogota" | jq .source
# → "cache"       (within the TTL window)
```

### 5b. Bypass city → always external

```bash
for i in 1 2 3; do
  curl -sS "http://${APP_IP}/api/weather?city=sao-paulo" | jq .source
done
# → "bypass" "bypass" "bypass"
```

### 5c. Unknown city → 4xx, no external call

```bash
curl -sS -w "%{http_code}\n" "http://${APP_IP}/api/weather?city=atlantis"
# → {"error":"unknown_city",...}404
```

### 5d. Open the UI

Open `http://${APP_IP}/` in a browser. Pick a city, click "Consultar",
verify the green/orange/red badge.

## 6. Run the load generator (the eBPF moment)

From your laptop or the cluster host:

```bash
make loadgen URL=http://${APP_IP}
```

Defaults: 30 req/s for 60 s, 90% across non-bypass cities, 10% to São Paulo.

To run it **inside the cluster** so Hubble shows pod-to-pod flows for the
loadgen Pod itself:

```bash
URL=http://weather-app.weather-demo.svc.cluster.local kubectl apply -f loadgen/loadgen.yaml
kubectl -n weather-demo logs -f job/loadgen
```

While the loadgen runs, switch to the eBPF tooling (Hubble UI / CLI) — the
flows you'll see:

- non-bypass cities: app Pod ↔ redis Pod (intra-cluster).
- `sao-paulo`: app Pod → `api.open-meteo.com:443` (egress).

That asymmetry is the punchline.

## 7. Reset demo state

To wipe the cache (e.g. before re-doing scenario 5a in front of the
audience):

```bash
make reset            # = kubectl -n weather-demo rollout restart deployment/redis
```

Wait ~5 s. Next call to any non-bypass city returns `"source":"origin"` again.

## 8. Cross-check metrics (optional, useful for live Q&A)

```bash
kubectl -n weather-demo port-forward svc/weather-app 8080:80 &
curl -sS http://localhost:8080/metrics | grep -E '^weather_(requests|origin)'
```

Sanity check during the talk:

- `sum(weather_origin_requests_total{city="sao-paulo"})` should grow
  monotonically with every São Paulo request.
- `sum(weather_origin_requests_total)` for any other city should equal the
  number of unique TTL windows that elapsed during the demo (≈ 1 per city
  for a single-talk window).

## 9. Tear down

```bash
make undeploy        # kubectl delete -k manifests/
```

This removes the namespace and everything inside it. The kind cluster
itself is left intact.

---

## Troubleshooting on stage

| Symptom                                                          | Most likely cause                                    | Remedy                                                                     |
|------------------------------------------------------------------|------------------------------------------------------|----------------------------------------------------------------------------|
| `EXTERNAL-IP <pending>` forever                                  | MetalLB pool exhausted or speaker pod down          | `kubectl -n metallb-system get pods`; or fall back to `port-forward`.       |
| All cities show `BYPASS` badge                                   | App can't reach Redis                               | `kubectl -n weather-demo logs deploy/weather-app` → look for `cache_error`. |
| All cities return 502                                            | Open-Meteo outage or no egress from cluster         | Test from inside a Pod: `kubectl run -it --rm curl --image=curlimages/curl --restart=Never -- curl -sS https://api.open-meteo.com/v1/forecast?latitude=4.71&longitude=-74.07&current_weather=true`. |
| Loadgen hits but no badge change in UI                           | Browser cached an old badge state                   | Soft-reload the page; the JS re-fetches on click anyway.                    |
| `make image` complains about missing `go.sum`                     | First build, Go modules not downloaded yet           | `go mod tidy` once locally before `make image`.                            |

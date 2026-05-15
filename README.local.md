# Run locally (no Docker, no Kubernetes)

```bash
# Sin Redis — arranca en modo degradado (source siempre origin/bypass)
REDIS_ADDR=localhost:1 LISTEN_ADDR=:8080 go run ./cmd/weather-app/

# Con Redis via Docker
docker run -d --rm -p 6379:6379 redis:7-alpine
REDIS_ADDR=localhost:6379 LISTEN_ADDR=:8080 go run ./cmd/weather-app/
```

```bash
# Consultar
curl "http://localhost:8080/api/weather?city=bogota"    # source: origin → cache
curl "http://localhost:8080/api/weather?city=sao-paulo" # source: bypass (siempre)
curl "http://localhost:8080/healthz"
curl "http://localhost:8080/metrics"

# Dashboard
open http://localhost:8080/
```

```bash
# Tests
make test
```

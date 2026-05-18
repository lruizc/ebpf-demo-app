# Run locally (no Docker, no Kubernetes)

```bash
# Without Redis — starts in degraded mode (source is always origin/bypass)
REDIS_ADDR=localhost:1 LISTEN_ADDR=:8080 go run ./cmd/weather-app/

# With Redis via Docker
docker run -d --rm -p 6379:6379 redis:7-alpine
REDIS_ADDR=localhost:6379 LISTEN_ADDR=:8080 go run ./cmd/weather-app/
```

```bash
# Query the API
curl "http://localhost:8080/api/weather?city=bogota"    # source: origin → cache
curl "http://localhost:8080/api/weather?city=sao-paulo" # source: bypass (always)
curl "http://localhost:8080/healthz"
curl "http://localhost:8080/metrics"

# Open the dashboard
open http://localhost:8080/
```

```bash
# Run tests
make test
```

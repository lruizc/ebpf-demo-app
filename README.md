# weather-app — eBPF talk demo

Un binario Go único que sirve un dashboard del clima para ciudades sudamericanas,
respaldado por un caché Redis. Una ciudad (`sao-paulo` por defecto) está configurada
para **siempre bypassear el caché**, produciendo un flujo constante de tráfico HTTPS
externo — el objetivo perfecto para aislar con eBPF / Hubble durante una charla en vivo.

> Especificación completa, plan y quickstart: [`specs/001-weather-cache-demo/`](specs/001-weather-cache-demo/)

---

## Arquitectura

```
Browser / curl
     │  GET /api/weather?city=bogota
     ▼
┌─────────────────────────────────────┐
│  weather-app Pod  (single binary)   │
│                                     │
│  handler → Decide(cfg, slug, cache) │
│      │                              │
│   cache ──── Redis Pod ─────────────┼── ClusterIP (solo tráfico interno)
│   origin/bypass                     │
│      │                              │
│      └─── api.open-meteo.com ───────┼── EGRESS (solo en miss o bypass)
└─────────────────────────────────────┘
         ▲
    MetalLB LoadBalancer IP   ← audiencia + speaker + loadgen
```

El campo `source` en cada respuesta JSON (`cache` / `origin` / `bypass`) es la
señal que eBPF/Hubble confirma a nivel de kernel.

**Badges en el dashboard:**

| Badge | Color | Significado eBPF |
|-------|-------|-----------------|
| `cache HIT (internal)` | 🟢 Verde | Cero paquetes hacia internet |
| `cache MISS (external)` | 🟠 Naranja | Un flujo saliente a Open-Meteo |
| `BYPASS (always external)` | 🔴 Rojo | Flujo saliente siempre visible |

---

## Dónde se ejecuta cada comando

> **Todos los comandos de despliegue corren en la torre Linux** donde está corriendo
> el cluster kind. La Mac solo sirve para editar código.

| Comando | Dónde corre |
|---|---|
| `make build` / `make test` | Cualquier máquina con Go instalado |
| `make image` | Torre Linux (necesita Docker) |
| `make load` | Torre Linux (necesita `kind` CLI) |
| `make deploy` / `make wait` / `make lb-ip` | Torre Linux (necesita `kubectl`) |
| `make loadgen` / `make reset` / `make logs` | Torre Linux |

## Quickstart (en la torre Linux)

Ver la guía paso a paso: [`specs/001-weather-cache-demo/quickstart.md`](specs/001-weather-cache-demo/quickstart.md)

```bash
# 1. Clonar el repo en la torre Linux
git clone lruizc.github.com:lruizc/ebpf-demo-app.git
cd ebpf-demo-app
git checkout 001-weather-cache-demo

# 2. Construir la imagen Docker (tag: weather-app:dev)
make image

# 3. Cargar la imagen en kind
#    Si tu cluster tiene un nombre diferente a "kind":
#    make load KIND_CLUSTER=<nombre>
make load

# 4. Desplegar en Kubernetes
make deploy
make wait      # espera Redis + weather-app Available

# 5. Obtener la IP y abrir el dashboard
make lb-ip     # ej: 192.168.64.100
# Abrir http://<IP>/ en el browser
```

**Verificación rápida:**
```bash
APP_IP=$(make -s lb-ip)
curl "http://${APP_IP}/api/weather?city=bogota"    # → "source":"origin"
curl "http://${APP_IP}/api/weather?city=bogota"    # → "source":"cache"
curl "http://${APP_IP}/api/weather?city=sao-paulo" # → "source":"bypass"
```

---

## Make targets

| Target | Descripción |
|---|---|
| `make build` | Compila el binario localmente |
| `make test` | Corre los tests unitarios (race detector) |
| `make image` | Construye la imagen Docker (`weather-app:dev`) |
| `make load` | Carga la imagen en kind |
| `make deploy` | `kubectl apply -k manifests/` |
| `make undeploy` | `kubectl delete -k manifests/` |
| `make wait` | Espera a que ambos Deployments estén Available |
| `make lb-ip` | Imprime la IP de MetalLB |
| `make loadgen` | Generador de carga local (90/10) |
| `make loadgen-job` | Corre el Job de carga dentro del cluster y sigue los logs |
| `make loadgen-clean` | Elimina el Job y su ConfigMap |
| `make reset` | Reinicia Redis para limpiar el caché |
| `make logs` | Sigue los logs estructurados JSON del app |
| `make help` | Muestra todos los targets |

---

## Demo runbook (secuencia durante la charla)

> Todos los comandos se ejecutan en la **torre Linux** desde el directorio del repo.

### 1. Pre-talk — preparar el cluster (en la torre Linux)

```bash
make image && make load && make deploy && make wait
APP_IP=$(make -s lb-ip)
echo "Dashboard: http://${APP_IP}/"
```

### 2. Mostrar el dashboard en el proyector

Abrir `http://<APP_IP>/` en el browser del presenter.

```bash
# Verificar los tres estados:
curl -s "http://${APP_IP}/api/weather?city=bogota"    # source: origin (primera vez)
curl -s "http://${APP_IP}/api/weather?city=bogota"    # source: cache  (dentro del TTL)
curl -s "http://${APP_IP}/api/weather?city=sao-paulo" # source: bypass (siempre)
```

### 3. Iniciar el generador de carga (eBPF moment)

En una terminal separada — esto produce el tráfico que Hubble/Cilium observa:

```bash
# Opción A: local contra MetalLB
APP_IP=$(make -s lb-ip) make loadgen

# Opción B: Job dentro del cluster (recomendado para la charla)
make loadgen-job
```

El generador produce ~90% caché hits (tráfico interno) y ~10% bypass hacia
`api.open-meteo.com` (tráfico externo visible en eBPF).

### 4. Mostrar Hubble / eBPF

Con el loadgen corriendo, en otra terminal filtrar los flujos:

```bash
# Hubble CLI (si está instalado)
hubble observe --pod weather-demo/weather-app --follow

# Solo flujos externos
hubble observe --pod weather-demo/weather-app \
  --verdict FORWARDED --follow \
  | grep -v "redis"
```

**Expectativa:** Los únicos flujos externos son hacia `api.open-meteo.com`
y coinciden 1:1 con requests de `sao-paulo`.

### 5. Verificar con métricas Prometheus

```bash
curl -s "http://${APP_IP}/metrics" \
  | grep weather_origin_requests_total
# weather_origin_requests_total{city="sao-paulo",outcome="success"} X  ← crece
# weather_origin_requests_total{city="bogota",outcome="success"}    1  ← solo 1
```

---

## Reset durante la charla

Si el demo se desincroniza o quieres mostrar el estado "frío" de nuevo:

```bash
make reset
```

Este comando hace `kubectl rollout restart deployment/redis`, lo que:

- **Limpia todo el caché** (el pod de Redis es efímero, sin PVC)
- **No reinicia el pod de `weather-app`** — los contadores Prometheus y el ciclo
  de vida del proceso continúan sin interrupciones (útil para correlacionar con
  capturas eBPF que usan el PID del proceso)
- El downtime de Redis es ~3-5 segundos; durante ese tiempo el app opera
  en modo degradado (sin caché) y sigue respondiendo

---

## Acceso de la audiencia desde la LAN

La IP de MetalLB es accesible desde cualquier dispositivo en la misma red:

```bash
make lb-ip    # imprime la IP, ej: 192.168.64.100
```

Desde el celular o laptop de la audiencia:

```
http://192.168.64.100/
```

No hay autenticación ni rate limit. Un cache miss desde la audiencia genera
como máximo una llamada extra a Open-Meteo por ciudad por hora — comportamiento
aceptable e incluso útil para enriquecer la visualización en Hubble.

---

## Configuración (env vars / ConfigMap)

| Clave | Default | Descripción |
|---|---|---|
| `BYPASS_CITIES` | `sao-paulo` | Slugs separados por coma que siempre van al origen |
| `TTL_SECONDS` | `3600` | TTL del caché en segundos |
| `REDIS_ADDR` | `redis:6379` | Host:port de Redis |
| `OPEN_METEO_URL` | `https://api.open-meteo.com/v1/forecast` | URL base del API de clima |
| `LISTEN_ADDR` | `:8080` | Dirección HTTP de escucha |
| `LOG_LEVEL` | `info` | Nivel de log: debug/info/warn/error |
| `CITIES` | *(8 ciudades SA)* | Override JSON del catálogo de ciudades |

Modificar `manifests/configmap.yaml` para cambiar la config en Kubernetes.

---

## Troubleshooting

```bash
# App no responde
kubectl -n weather-demo get pods
kubectl -n weather-demo describe pod -l app=weather-app

# Redis no conecta
kubectl -n weather-demo logs -l app=redis
kubectl -n weather-demo exec -it deploy/redis -- redis-cli ping

# IP de MetalLB vacía
kubectl -n weather-demo get svc weather-app   # revisar EXTERNAL-IP

# Logs JSON estructurados
make logs | jq .
```

---

## Constitución (v1.0.1)

Principios no negociables: [`.specify/memory/constitution.md`](.specify/memory/constitution.md)

Los únicos dials TCP externos del proceso son:
1. `redis:6379` — lecturas/escrituras de caché
2. `api.open-meteo.com` — fetch de clima (solo en miss o bypass)

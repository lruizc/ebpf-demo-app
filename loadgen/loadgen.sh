#!/usr/bin/env bash
# loadgen.sh — continuous 90/10 weather-app load generator
#
# Usage:  bash loadgen.sh [BASE_URL]
#   BASE_URL defaults to http://localhost:8080
#
# Traffic mix:
#   90% → random city from ALL_CITIES (mostly cache hits after first request)
#   10% → sao-paulo (always external — eBPF demo target)
#
# Requires: curl, shuf (GNU coreutils)

set -euo pipefail

BASE_URL="${1:-http://localhost:8080}"

ALL_CITIES=(bogota lima santiago buenos-aires caracas quito montevideo sao-paulo)
BYPASS_CITY="sao-paulo"

INTERVAL="${LOADGEN_INTERVAL_MS:-500}"  # ms between requests
TOTAL="${LOADGEN_TOTAL:-0}"             # 0 = run indefinitely
count=0

echo "[loadgen] Starting against ${BASE_URL}"
echo "[loadgen] interval=${INTERVAL}ms  total=${TOTAL:-∞}"
echo "[loadgen] Ctrl-C to stop"
echo ""

while true; do
  # Decide city: every ~10th request goes to the bypass city
  rand=$((RANDOM % 10))
  if [ "$rand" -eq 0 ]; then
    city="$BYPASS_CITY"
  else
    city="${ALL_CITIES[$((RANDOM % ${#ALL_CITIES[@]}))]}"
  fi

  source=$(curl -s -o /dev/null \
    -w "%header{X-Source}" \
    "${BASE_URL}/api/weather?city=${city}" 2>/dev/null || echo "error")

  echo "[loadgen] city=${city}  source=${source}"

  count=$((count + 1))
  if [ "$TOTAL" -gt 0 ] && [ "$count" -ge "$TOTAL" ]; then
    echo "[loadgen] Done — sent ${count} requests"
    exit 0
  fi

  sleep "$(echo "scale=3; ${INTERVAL}/1000" | bc)"
done

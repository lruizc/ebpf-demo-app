#!/usr/bin/env bash
# loadgen.sh — weather-app load generator for the eBPF demo
#
# Usage:
#   bash loadgen.sh [BASE_URL]
#
# Environment variables:
#   BASE_URL      — target base URL            (default: http://localhost:8080)
#   RATE          — requests per second        (default: 2)
#   DURATION      — run duration in seconds    (default: 0 = run forever)
#   BYPASS_RATIO  — % of requests to bypass city (default: 10)
#
# Output: one line per request; summary every 10 s with per-source counts.
#
# Requires: curl, bc

set -euo pipefail

BASE_URL="${1:-${BASE_URL:-http://localhost:8080}}"
RATE="${RATE:-2}"
DURATION="${DURATION:-0}"
BYPASS_RATIO="${BYPASS_RATIO:-10}"

ALL_CITIES=(bogota lima santiago buenos-aires caracas quito montevideo sao-paulo)
BYPASS_CITY="sao-paulo"

# Inter-request sleep in seconds (bc for floating-point)
SLEEP_S=$(echo "scale=4; 1 / ${RATE}" | bc)

# Per-source counters
cnt_cache=0
cnt_origin=0
cnt_bypass=0
cnt_error=0
total=0
start_ts=$(date +%s)
last_summary_ts=$start_ts

echo "[loadgen] target=${BASE_URL}"
echo "[loadgen] rate=${RATE} rps  duration=${DURATION}s (0=∞)  bypass_ratio=${BYPASS_RATIO}%"
echo "[loadgen] Ctrl-C to stop"
echo ""

print_summary() {
  local now
  now=$(date +%s)
  local elapsed=$(( now - start_ts ))
  echo "──────────────────────────────────────────────────────"
  echo "[summary] elapsed=${elapsed}s total=${total}"
  echo "          cache=${cnt_cache}  origin=${cnt_origin}  bypass=${cnt_bypass}  error=${cnt_error}"
  echo "──────────────────────────────────────────────────────"
}

trap 'echo ""; print_summary; exit 0' INT TERM

while true; do
  # Check duration limit
  if [ "${DURATION}" -gt 0 ]; then
    now=$(date +%s)
    if [ $(( now - start_ts )) -ge "${DURATION}" ]; then
      print_summary
      echo "[loadgen] Duration reached — exiting"
      exit 0
    fi
  fi

  # Select city: BYPASS_RATIO% chance of bypass city
  rand=$(( RANDOM % 100 ))
  if [ "$rand" -lt "$BYPASS_RATIO" ]; then
    city="$BYPASS_CITY"
  else
    city="${ALL_CITIES[$((RANDOM % ${#ALL_CITIES[@]}))]}"
  fi

  # Fire the request and capture X-Source header
  source=$(curl -sf -o /dev/null \
    -w "%header{X-Source}" \
    --max-time 5 \
    "${BASE_URL}/api/weather?city=${city}" 2>/dev/null || echo "error")

  case "$source" in
    cache)   cnt_cache=$(( cnt_cache + 1 ))   ;;
    origin)  cnt_origin=$(( cnt_origin + 1 ))  ;;
    bypass)  cnt_bypass=$(( cnt_bypass + 1 ))  ;;
    *)       cnt_error=$(( cnt_error + 1 )); source="error" ;;
  esac
  total=$(( total + 1 ))

  echo "[loadgen] city=${city}  source=${source}"

  # Print summary every 10 s
  now=$(date +%s)
  if [ $(( now - last_summary_ts )) -ge 10 ]; then
    print_summary
    last_summary_ts=$now
  fi

  sleep "$SLEEP_S"
done

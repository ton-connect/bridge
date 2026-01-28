#!/usr/bin/env bash
set -euo pipefail

# Simple launcher for k6 benchmark in Docker, without docker-compose.
# It can run one or multiple instances by setting TOTAL_INSTANCES.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE="${IMAGE:-k6-benchmark:latest}"

# Bridge configuration
PORT="${PORT:-8081}"
BRIDGE_URL_DEFAULT="http://localhost:${PORT}/bridge"
BRIDGE_URL="${BRIDGE_URL:-$BRIDGE_URL_DEFAULT}"
AUTH_TOKEN="${AUTH_TOKEN:-test-token}"

# Load configuration (defaults match bridge_test.js)
SSE_VUS="${SSE_VUS:-1000}"
SEND_RATE="${SEND_RATE:-10}"
LISTENER_WRITERS_RATIO="${LISTENER_WRITERS_RATIO:-3}"
TOTAL_INSTANCES="${TOTAL_INSTANCES:-1}"
ID_SPACE_OFFSET="${ID_SPACE_OFFSET:-0}"

SSE_RAMP_UP="${SSE_RAMP_UP:-10s}"
SSE_HOLD="${SSE_HOLD:-50s}"
SSE_RAMP_DOWN="${SSE_RAMP_DOWN:-10s}"
SSE_DELAY="${SSE_DELAY:-0s}"

SENDER_RAMP_UP="${SENDER_RAMP_UP:-10s}"
SENDER_HOLD="${SENDER_HOLD:-30s}"
SENDER_RAMP_DOWN="${SENDER_RAMP_DOWN:-10s}"
SENDER_DELAY="${SENDER_DELAY:-10s}"

RESULTS_DIR="${SCRIPT_DIR}/results"
mkdir -p "${RESULTS_DIR}"

echo "============================================================"
echo "Running k6 benchmark in Docker"
echo "Image:            ${IMAGE}"
echo "Bridge URL:       ${BRIDGE_URL}"
echo "Auth token:       ${AUTH_TOKEN}"
echo "SSE_VUS:          ${SSE_VUS}"
echo "SEND_RATE:        ${SEND_RATE}"
echo "LISTENER_RATIO:   ${LISTENER_WRITERS_RATIO}"
echo "TOTAL_INSTANCES:  ${TOTAL_INSTANCES}"
echo "Results dir:      ${RESULTS_DIR}"
echo "============================================================"

for ((i=0; i< TOTAL_INSTANCES; i++)); do
  echo ""
  echo ">>> Starting instance ${i}/${TOTAL_INSTANCES} ..."

  docker run --rm \
    -e BRIDGE_URL="${BRIDGE_URL}" \
    -e AUTH_TOKEN="${AUTH_TOKEN}" \
    -e SSE_VUS="${SSE_VUS}" \
    -e SEND_RATE="${SEND_RATE}" \
    -e LISTENER_WRITERS_RATIO="${LISTENER_WRITERS_RATIO}" \
    -e TOTAL_INSTANCES="${TOTAL_INSTANCES}" \
    -e CURRENT_INSTANCE="${i}" \
    -e ID_SPACE_OFFSET="${ID_SPACE_OFFSET}" \
    -e SSE_RAMP_UP="${SSE_RAMP_UP}" \
    -e SSE_HOLD="${SSE_HOLD}" \
    -e SSE_RAMP_DOWN="${SSE_RAMP_DOWN}" \
    -e SSE_DELAY="${SSE_DELAY}" \
    -e SENDER_RAMP_UP="${SENDER_RAMP_UP}" \
    -e SENDER_HOLD="${SENDER_HOLD}" \
    -e SENDER_RAMP_DOWN="${SENDER_RAMP_DOWN}" \
    -e SENDER_DELAY="${SENDER_DELAY}" \
    -v "${RESULTS_DIR}:/results" \
    "${IMAGE}" \
    run --summary-export="/results/summary-${i}.json" /scripts/bridge_test.js &
done

echo ""
echo "Waiting for all instances to finish..."
wait
echo "All instances finished. Summaries are in: ${RESULTS_DIR}"



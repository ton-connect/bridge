#!/usr/bin/env bash
set -euo pipefail

# Directory with k6 summary JSON files (default: ./results)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${1:-${SCRIPT_DIR}/results}"

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required but not installed. Please install jq and retry." >&2
  exit 1
fi

if [ ! -d "${RESULTS_DIR}" ]; then
  echo "ERROR: results directory not found: ${RESULTS_DIR}" >&2
  exit 1
fi

shopt -s nullglob
FILES=( "${RESULTS_DIR}"/summary-*.json )
shopt -u nullglob

if [ ${#FILES[@]} -eq 0 ]; then
  echo "No summary-*.json files found in ${RESULTS_DIR}" >&2
  exit 1
fi

total_sent=0
total_received=0

echo "Summarizing k6 results in: ${RESULTS_DIR}"
echo "Files:"
for f in "${FILES[@]}"; do
  echo "  - $(basename "$f")"
done
echo

for f in "${FILES[@]}"; do
  sent=$(jq -r '.metrics.sse_message_sent.count // 0' "$f")
  received=$(jq -r '.metrics.sse_message_received.count // 0' "$f")

  echo "$(basename "$f"): sse_message_sent.count=${sent}, sse_message_received.count=${received}"

  total_sent=$(( total_sent + sent ))
  total_received=$(( total_received + received ))
done

echo
echo "========================================="
echo "TOTAL sse_message_sent.count:      ${total_sent}"
echo "TOTAL sse_message_received.count:  ${total_received}"
echo "========================================="



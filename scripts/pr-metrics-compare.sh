#!/bin/bash
set -e

# Usage: pr-metrics-compare.sh <metrics-dir>
# Reads JSON metrics files from <metrics-dir> and generates comparison tables.
# Expected files: <metrics-dir>/metrics-<storage>-pr/metrics-<storage>-pr.json
#                 <metrics-dir>/metrics-<storage>-base/metrics-<storage>-base.json

METRICS_DIR="${1:-.}"
STORAGES="memory postgres nginx cluster-valkey dnsmasq"

pct_change() {
  local base="$1" pr="$2"
  if [ "$base" = "0" ] || [ -z "$base" ]; then
    if [ "$pr" = "0" ] || [ -z "$pr" ]; then
      echo "—"
    else
      echo "+∞"
    fi
    return
  fi
  local diff
  diff=$(awk "BEGIN {d = (($pr - $base) / $base) * 100; printf \"%.1f\", d}" 2>/dev/null || echo "0.0")
  if [ "$diff" = "0.0" ] || [ "$diff" = "-0.0" ]; then
    echo "—"
  elif echo "$diff" | grep -q "^-"; then
    echo "${diff}%"
  else
    echo "+${diff}%"
  fi
}

json_val() {
  local file="$1" field="$2"
  if [ -f "$file" ]; then
    python3 -c "import json,sys; d=json.load(open('$file')); print(d.get('$field','0'))" 2>/dev/null || echo "0"
  else
    echo "—"
  fi
}

row() {
  local label="$1" base_val="$2" pr_val="$3" suffix="$4"
  echo "| $label | ${base_val}${suffix} | ${pr_val}${suffix} | $(pct_change "$base_val" "$pr_val") |"
}

first=true
for storage in $STORAGES; do
  pr_file="$METRICS_DIR/metrics-${storage}-pr/metrics-${storage}-pr.json"
  base_file="$METRICS_DIR/metrics-${storage}-base/metrics-${storage}-base.json"

  if [ ! -f "$pr_file" ] && [ ! -f "$base_file" ]; then
    continue
  fi

  if [ "$first" = true ]; then
    first=false
  else
    echo ""
  fi

  echo "**${storage}**"
  echo ""
  echo "| Metric | master | PR | Δ |"
  echo "|--------|--------|-----|---|"
  row "CPU"         "$(json_val "$base_file" cpu_seconds)"   "$(json_val "$pr_file" cpu_seconds)"   "s"
  row "Goroutines"  "$(json_val "$base_file" goroutines)"    "$(json_val "$pr_file" goroutines)"    ""
  row "Threads"     "$(json_val "$base_file" threads)"       "$(json_val "$pr_file" threads)"       ""
  row "Heap"        "$(json_val "$base_file" heap_mb)"       "$(json_val "$pr_file" heap_mb)"       "MB"
  row "RAM"         "$(json_val "$base_file" rss_mb)"        "$(json_val "$pr_file" rss_mb)"        "MB"
  row "Total Alloc" "$(json_val "$base_file" total_alloc_mb)" "$(json_val "$pr_file" total_alloc_mb)" "MB"
  row "Allocs"      "$(json_val "$base_file" allocs_count)"  "$(json_val "$pr_file" allocs_count)"  ""
  row "GC Cycles"   "$(json_val "$base_file" gc_cycles)"     "$(json_val "$pr_file" gc_cycles)"     ""
  row "GC Avg"      "$(json_val "$base_file" gc_avg_ms)"     "$(json_val "$pr_file" gc_avg_ms)"     "ms"
  row "FDs"         "$(json_val "$base_file" open_fds)"      "$(json_val "$pr_file" open_fds)"      ""
done

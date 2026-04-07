#!/bin/bash
set -e

# Usage: pr-metrics-compare.sh <metrics-dir>
# Reads JSON metrics files from <metrics-dir> and generates a comparison table.
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

jq_val() {
  local file="$1" field="$2"
  if [ -f "$file" ]; then
    python3 -c "import json,sys; d=json.load(open('$file')); print(d.get('$field','0'))" 2>/dev/null || echo "0"
  else
    echo "—"
  fi
}

echo "| Storage | Branch | CPU | Goroutines | Threads | Heap | RAM | Total Alloc | Allocs | GC Cycles | GC Avg | FDs |"
echo "|---------|--------|-----|------------|---------|------|-----|-------------|--------|-----------|--------|-----|"

for storage in $STORAGES; do
  pr_file="$METRICS_DIR/metrics-${storage}-pr/metrics-${storage}-pr.json"
  base_file="$METRICS_DIR/metrics-${storage}-base/metrics-${storage}-base.json"

  if [ ! -f "$pr_file" ] && [ ! -f "$base_file" ]; then
    continue
  fi

  pr_cpu=$(jq_val "$pr_file" "cpu_seconds")
  base_cpu=$(jq_val "$base_file" "cpu_seconds")
  pr_goroutines=$(jq_val "$pr_file" "goroutines")
  base_goroutines=$(jq_val "$base_file" "goroutines")
  pr_threads=$(jq_val "$pr_file" "threads")
  base_threads=$(jq_val "$base_file" "threads")
  pr_heap=$(jq_val "$pr_file" "heap_mb")
  base_heap=$(jq_val "$base_file" "heap_mb")
  pr_rss=$(jq_val "$pr_file" "rss_mb")
  base_rss=$(jq_val "$base_file" "rss_mb")
  pr_total=$(jq_val "$pr_file" "total_alloc_mb")
  base_total=$(jq_val "$base_file" "total_alloc_mb")
  pr_allocs=$(jq_val "$pr_file" "allocs_count")
  base_allocs=$(jq_val "$base_file" "allocs_count")
  pr_gc=$(jq_val "$pr_file" "gc_cycles")
  base_gc=$(jq_val "$base_file" "gc_cycles")
  pr_gc_avg=$(jq_val "$pr_file" "gc_avg_ms")
  base_gc_avg=$(jq_val "$base_file" "gc_avg_ms")
  pr_fds=$(jq_val "$pr_file" "open_fds")
  base_fds=$(jq_val "$base_file" "open_fds")

  echo "| **${storage}** | master | ${base_cpu}s | ${base_goroutines} | ${base_threads} | ${base_heap}MB | ${base_rss}MB | ${base_total}MB | ${base_allocs} | ${base_gc} | ${base_gc_avg}ms | ${base_fds} |"
  echo "| | PR | ${pr_cpu}s | ${pr_goroutines} | ${pr_threads} | ${pr_heap}MB | ${pr_rss}MB | ${pr_total}MB | ${pr_allocs} | ${pr_gc} | ${pr_gc_avg}ms | ${pr_fds} |"
  echo "| | **Δ** | $(pct_change "$base_cpu" "$pr_cpu") | $(pct_change "$base_goroutines" "$pr_goroutines") | $(pct_change "$base_threads" "$pr_threads") | $(pct_change "$base_heap" "$pr_heap") | $(pct_change "$base_rss" "$pr_rss") | $(pct_change "$base_total" "$pr_total") | $(pct_change "$base_allocs" "$pr_allocs") | $(pct_change "$base_gc" "$pr_gc") | $(pct_change "$base_gc_avg" "$pr_gc_avg") | $(pct_change "$base_fds" "$pr_fds") |"
done

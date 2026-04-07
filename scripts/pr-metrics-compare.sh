#!/bin/bash
set -e

# Usage: pr-metrics-compare.sh <metrics-dir>
# Reads averaged JSON metrics and generates collapsed comparison tables with summaries.
# Expected files: <metrics-dir>/metrics-<storage>-pr/metrics-<storage>-pr.json
#                 <metrics-dir>/metrics-<storage>-base/metrics-<storage>-base.json

METRICS_DIR="${1:-.}"
STORAGES="memory postgres nginx cluster-valkey dnsmasq"
THRESHOLD=50

json_val() {
  local file="$1" field="$2"
  if [ -f "$file" ]; then
    python3 -c "import json; d=json.load(open('$file')); print(d.get('$field','0'))" 2>/dev/null || echo "0"
  else
    echo "—"
  fi
}

raw_pct() {
  local base="$1" pr="$2"
  if [ "$base" = "0" ] || [ -z "$base" ]; then
    echo "0"
    return
  fi
  awk "BEGIN {printf \"%.1f\", (($pr - $base) / $base) * 100}" 2>/dev/null || echo "0"
}

fmt_pct() {
  local val="$1"
  if [ "$val" = "0" ] || [ "$val" = "0.0" ] || [ "$val" = "-0.0" ]; then
    echo "—"
  elif echo "$val" | grep -q "^-"; then
    echo "${val}%"
  else
    echo "+${val}%"
  fi
}

# metric definitions: "key|label|suffix"
METRICS="cpu_seconds|CPU|s
goroutines|Goroutines|
threads|Threads|
heap_mb|Heap|MB
rss_mb|RAM|MB
total_alloc_mb|Total Alloc|MB
allocs_count|Allocs|
gc_cycles|GC Cycles|
gc_avg_ms|GC Avg|ms
open_fds|FDs|"

first=true
for storage in $STORAGES; do
  pr_file="$METRICS_DIR/metrics-${storage}-pr/metrics-${storage}-pr.json"
  base_file="$METRICS_DIR/metrics-${storage}-base/metrics-${storage}-base.json"

  if [ ! -f "$pr_file" ] && [ ! -f "$base_file" ]; then
    continue
  fi

  # Build summary
  summary_parts=""
  while IFS='|' read -r key label suffix; do
    bv=$(json_val "$base_file" "$key")
    pv=$(json_val "$pr_file" "$key")
    pct=$(raw_pct "$bv" "$pv")
    abs_pct=$(echo "$pct" | sed 's/^-//')
    over=$(awk "BEGIN {print ($abs_pct >= $THRESHOLD) ? 1 : 0}" 2>/dev/null || echo "0")
    if [ "$over" = "1" ]; then
      if [ -n "$summary_parts" ]; then
        summary_parts="$summary_parts, $label $(fmt_pct "$pct")"
      else
        summary_parts="$label $(fmt_pct "$pct")"
      fi
    fi
  done <<< "$METRICS"

  if [ -z "$summary_parts" ]; then
    summary_tag="✅"
  else
    summary_tag="⚠️ $summary_parts"
  fi

  if [ "$first" = true ]; then
    first=false
  else
    echo ""
  fi

  echo "<details>"
  echo "<summary><b>${storage}</b> — ${summary_tag}</summary>"
  echo ""
  echo "| Metric | main | PR | Δ |"
  echo "|--------|--------|-----|---|"

  while IFS='|' read -r key label suffix; do
    bv=$(json_val "$base_file" "$key")
    pv=$(json_val "$pr_file" "$key")
    pct=$(raw_pct "$bv" "$pv")
    echo "| $label | ${bv}${suffix} | ${pv}${suffix} | $(fmt_pct "$pct") |"
  done <<< "$METRICS"

  echo ""
  echo "</details>"
done

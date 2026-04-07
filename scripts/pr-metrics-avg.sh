#!/bin/bash
set -e

# Runs the bridge N times, collects JSON metrics each time, and outputs the averaged result.
# Usage: pr-metrics-avg.sh <storage> <compose-file> <runs> <metrics-script>

STORAGE="${1:?usage: pr-metrics-avg.sh <storage> <compose-file> <runs> <metrics-script>}"
COMPOSE_FILE="${2:?}"
RUNS="${3:-5}"
METRICS_SCRIPT="${4:-./scripts/pr-metrics.sh}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

for i in $(seq 1 "$RUNS"); do
  echo "=== Run $i/$RUNS ($STORAGE) ===" >&2

  docker compose -f "$COMPOSE_FILE" up --build -d bridge >&2 2>&1

  # wait for bridge to be ready
  for attempt in $(seq 1 30); do
    if curl -s "http://localhost:9103/metrics" > /dev/null 2>&1; then
      break
    fi
    sleep 1
  done

  docker compose -f "$COMPOSE_FILE" run --rm gointegration >&2 2>&1 || true

  "$METRICS_SCRIPT" "$STORAGE" localhost 9103 json > "$TMPDIR/run-${i}.json"

  docker compose -f "$COMPOSE_FILE" down -v >&2 2>&1
done

# Average all JSON files
python3 -c "
import json, glob, os

files = sorted(glob.glob(os.path.join('$TMPDIR', 'run-*.json')))
if not files:
    print('{}')
    exit()

all_data = [json.load(open(f)) for f in files]
n = len(all_data)

# non-numeric fields
result = {'storage': all_data[0].get('storage', '$STORAGE')}

# numeric fields to average
numeric_keys = [
    'cpu_seconds', 'cores', 'goroutines', 'threads',
    'heap_mb', 'rss_mb', 'total_alloc_mb', 'allocs_count',
    'gc_cycles', 'gc_avg_ms', 'open_fds', 'max_fds', 'fd_percent'
]

for key in numeric_keys:
    vals = []
    for d in all_data:
        try:
            vals.append(float(d.get(key, '0')))
        except (ValueError, TypeError):
            vals.append(0.0)
    avg = sum(vals) / n
    # format integers vs floats
    if key in ('cores', 'goroutines', 'threads', 'allocs_count', 'gc_cycles', 'open_fds', 'max_fds'):
        result[key] = str(int(round(avg)))
    else:
        result[key] = f'{avg:.2f}'

print(json.dumps(result))
"

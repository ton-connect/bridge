#!/bin/bash
set -e

STORAGE_TYPE="${1:-unknown}"
BRIDGE_HOST="${2:-localhost}"
BRIDGE_PORT="${3:-9103}"
OUTPUT_FORMAT="${4:-markdown}"

echo "Waiting for bridge service at $BRIDGE_HOST:$BRIDGE_PORT..." >&2
for i in {1..30}; do
  if curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/metrics" > /dev/null 2>&1; then
    echo "Bridge is ready!" >&2
    break
  fi
  if [ $i -eq 30 ]; then
    echo "📊 Performance Metrics ($STORAGE_TYPE storage)"
    echo ""
    echo "❌ **Bridge service not accessible after 30 seconds**"
    exit 0
  fi
  sleep 1
done

# Get goroutine count (need ?debug=1 for text output)
GOROUTINE_DATA=$(curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/debug/pprof/goroutine?debug=1" 2>/dev/null || echo "")
GOROUTINES=$(echo "$GOROUTINE_DATA" | head -1 | grep -o '[0-9]\+' | head -1 || echo "0")

# Get metrics data once and reuse
METRICS_DATA=$(curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/metrics" 2>/dev/null || echo "")

# CPU Metrics
PROCESS_CPU_TOTAL=$(echo "$METRICS_DATA" | grep "process_cpu_seconds_total" | grep -o '[0-9.]\+' | tail -1 || echo "0")
PROCESS_CPU_FORMATTED=$(echo "$PROCESS_CPU_TOTAL" | awk '{printf "%.2f", $1}' 2>/dev/null || echo "0.00")
GOMAXPROCS=$(echo "$METRICS_DATA" | grep "go_sched_gomaxprocs_threads" | grep -o '[0-9]\+' || echo "0")

# Memory Metrics
HEAP_BYTES=$(echo "$METRICS_DATA" | grep "go_memstats_heap_inuse_bytes" | grep -o '[0-9.e+-]\+' | tail -1 || echo "0")
HEAP_MB=$(echo "$HEAP_BYTES" | awk '{printf "%.1f", $1/1024/1024}' 2>/dev/null || echo "0.0")

HEAP_ALLOC_BYTES=$(echo "$METRICS_DATA" | grep "go_memstats_heap_alloc_bytes " | grep -o '[0-9.e+-]\+' | tail -1 || echo "0")
HEAP_ALLOC_MB=$(echo "$HEAP_ALLOC_BYTES" | awk '{printf "%.1f", $1/1024/1024}' 2>/dev/null || echo "0.0")

RSS_BYTES=$(echo "$METRICS_DATA" | grep "process_resident_memory_bytes" | grep -o '[0-9.e+-]\+' | tail -1 || echo "0")
RSS_MB=$(echo "$RSS_BYTES" | awk '{printf "%.1f", $1/1024/1024}' 2>/dev/null || echo "0.0")

TOTAL_ALLOCS=$(echo "$METRICS_DATA" | grep "go_memstats_alloc_bytes_total" | grep -o '[0-9.e+-]\+' | tail -1 || echo "0")
TOTAL_ALLOCS_MB=$(echo "$TOTAL_ALLOCS" | awk '{printf "%.1f", $1/1024/1024}' 2>/dev/null || echo "0.0")

# Garbage Collection Metrics
GC_COUNT=$(echo "$METRICS_DATA" | grep "go_gc_duration_seconds_count" | grep -o '[0-9]\+' || echo "0")
GC_TOTAL_TIME=$(echo "$METRICS_DATA" | grep "go_gc_duration_seconds_sum" | grep -o '[0-9.]\+' || echo "0")
GC_AVG_MS=$(echo "$GC_TOTAL_TIME $GC_COUNT" | awk '{if($2>0) printf "%.2f", ($1/$2)*1000; else print "0"}' 2>/dev/null || echo "0")

# Resource Metrics - robust cross-platform parsing
OPEN_FDS=$(echo "$METRICS_DATA" | awk '/^process_open_fds / {print int($2); exit}' || echo "0")
MAX_FDS=$(echo "$METRICS_DATA" | awk '/^process_max_fds / {print int($2); exit}' || echo "0")
FD_USAGE_PERCENT=$(echo "$OPEN_FDS $MAX_FDS" | awk '{if($2>0) printf "%.1f", ($1/$2)*100; else print "0.0"}' || echo "0.0")

# Get total allocation count from metrics (not pprof sampling)
ALLOCS_COUNT=$(echo "$METRICS_DATA" | awk '/^go_memstats_mallocs_total / {print int($2); exit}' || echo "0")

# Get thread count
THREADS_DATA=$(curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/debug/pprof/threadcreate?debug=1" 2>/dev/null || echo "")
THREADS=$(echo "$THREADS_DATA" | head -1 | grep -o 'total [0-9]\+' | grep -o '[0-9]\+' || echo "0")

# Output
if [ "$OUTPUT_FORMAT" = "json" ]; then
  cat <<ENDJSON
{"storage":"$STORAGE_TYPE","cpu_seconds":"$PROCESS_CPU_FORMATTED","cores":"$GOMAXPROCS","goroutines":"$GOROUTINES","threads":"$THREADS","heap_mb":"$HEAP_ALLOC_MB","rss_mb":"$RSS_MB","total_alloc_mb":"$TOTAL_ALLOCS_MB","allocs_count":"$ALLOCS_COUNT","gc_cycles":"$GC_COUNT","gc_avg_ms":"$GC_AVG_MS","open_fds":"$OPEN_FDS","max_fds":"$MAX_FDS","fd_percent":"$FD_USAGE_PERCENT"}
ENDJSON
else
  echo "**Performance Metrics** ($STORAGE_TYPE storage)"
  echo ""
  echo "- **CPU:** ${PROCESS_CPU_FORMATTED}s (${GOMAXPROCS} cores) • **Goroutines:** $GOROUTINES • **Threads:** $THREADS"
  echo "- **Memory:** ${HEAP_ALLOC_MB}MB heap • ${RSS_MB}MB RAM • ${TOTAL_ALLOCS_MB}MB total • $ALLOCS_COUNT allocs"
  echo "- **GC:** $GC_COUNT cycles (${GC_AVG_MS}ms avg)"
  echo "- **FDs:** $OPEN_FDS/$MAX_FDS (${FD_USAGE_PERCENT}%)"
  echo ""
fi

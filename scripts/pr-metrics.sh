#!/bin/bash
set -e

STORAGE_TYPE="${1:-unknown}"
BRIDGE_HOST="${2:-localhost}"
BRIDGE_PORT="${3:-9103}"

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

# Get heap size from metrics endpoint (handle scientific notation)
METRICS_DATA=$(curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/metrics" 2>/dev/null || echo "")
HEAP_BYTES=$(echo "$METRICS_DATA" | grep "go_memstats_heap_inuse_bytes" | grep -o '[0-9.e+-]\+' | tail -1 || echo "0")
HEAP_MB=$(echo "$HEAP_BYTES" | awk '{printf "%.1f", $1/1024/1024}' 2>/dev/null || echo "0.0")

# Get allocation rate from allocs endpoint
ALLOCS_DATA=$(curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/debug/pprof/allocs?debug=1" 2>/dev/null || echo "")
ALLOCS_COUNT=$(echo "$ALLOCS_DATA" | head -1 | grep -o '\[[0-9]\+:' | grep -o '[0-9]\+' | head -1 || echo "0")

# Get thread count
THREADS_DATA=$(curl -s "http://$BRIDGE_HOST:$BRIDGE_PORT/debug/pprof/threadcreate?debug=1" 2>/dev/null || echo "")
THREADS=$(echo "$THREADS_DATA" | head -1 | grep -o 'total [0-9]\+' | grep -o '[0-9]\+' || echo "0")

# Output markdown to stdout
echo "📊 Performance Metrics ($STORAGE_TYPE storage)"
echo ""
echo "- **Goroutines:** $GOROUTINES"
echo "- **OS Threads:** $THREADS"
echo "- **Heap Size:** ${HEAP_MB}MB"
echo "- **Allocations:** $ALLOCS_COUNT"
echo ""

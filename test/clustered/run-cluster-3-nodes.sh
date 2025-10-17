#!/bin/bash

# Script to run tests on 3-node Valkey cluster
# Usage: ./run-cluster-3-nodes.sh [write-heavy|read-heavy|mixed]

set -e

SCENARIO=${1:-mixed}
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.cluster-3-nodes.yml"

# Define test scenarios
case "$SCENARIO" in
  write-heavy)
    echo "Running WRITE-HEAVY scenario (75% writes, 25% reads)"
    export SSE_VUS=250
    export SEND_RATE=7500
    export TEST_DURATION=2m
    ;;
  read-heavy)
    echo "Running READ-HEAVY scenario (25% writes, 75% reads)"
    export SSE_VUS=7500
    export SEND_RATE=250
    export TEST_DURATION=2m
    ;;
  mixed)
    echo "Running MIXED scenario (50% writes, 50% reads)"
    export SSE_VUS=500
    export SEND_RATE=5000
    export TEST_DURATION=2m
    ;;
  *)
    echo "Unknown scenario: $SCENARIO"
    echo "Usage: $0 [write-heavy|read-heavy|mixed]"
    exit 1
    ;;
esac

# Create results directory
mkdir -p "$SCRIPT_DIR/results/cluster-3-nodes"

# Clean up any existing containers
echo "Cleaning up existing containers..."
docker-compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true

# Start the cluster
echo "Starting 3-node Valkey cluster..."
docker-compose -f "$COMPOSE_FILE" up -d valkey-shard1 valkey-shard2 valkey-shard3 valkey-cluster-init bridge data-monitor

# Wait for cluster to be ready
echo "Waiting for cluster initialization..."
sleep 15

# Check cluster status
echo "Checking cluster status..."
docker-compose -f "$COMPOSE_FILE" exec -T valkey-shard1 valkey-cli cluster info || true

# Run benchmark
echo ""
echo "========================================"
echo "Starting benchmark: $SCENARIO"
echo "SSE_VUS=$SSE_VUS"
echo "SEND_RATE=$SEND_RATE"
echo "TEST_DURATION=$TEST_DURATION"
echo "========================================"
echo ""

docker-compose -f "$COMPOSE_FILE" up --abort-on-container-exit benchmark

# Save test metadata
RESULT_FILE="$SCRIPT_DIR/results/cluster-3-nodes/test-metadata-$SCENARIO.txt"
echo "Test Scenario: $SCENARIO" > "$RESULT_FILE"
echo "Date: $(date)" >> "$RESULT_FILE"
echo "SSE_VUS: $SSE_VUS" >> "$RESULT_FILE"
echo "SEND_RATE: $SEND_RATE" >> "$RESULT_FILE"
echo "TEST_DURATION: $TEST_DURATION" >> "$RESULT_FILE"
echo "Cluster: 3-node (0.45 CPU total)" >> "$RESULT_FILE"

# Rename summary file to include scenario
if [ -f "$SCRIPT_DIR/results/cluster-3-nodes/summary.json" ]; then
  mv "$SCRIPT_DIR/results/cluster-3-nodes/summary.json" \
     "$SCRIPT_DIR/results/cluster-3-nodes/summary-$SCENARIO.json"
fi

echo ""
echo "========================================"
echo "Test completed: $SCENARIO"
echo "Results saved to: results/cluster-3-nodes/"
echo "========================================"
echo ""

# Show quick summary
if [ -f "$SCRIPT_DIR/results/cluster-3-nodes/summary-$SCENARIO.json" ]; then
  echo "Quick Summary:"
  cat "$SCRIPT_DIR/results/cluster-3-nodes/summary-$SCENARIO.json" | \
    grep -E '"http_req_failed"|"http_req_duration"|"delivery_latency"' || true
fi

# Keep cluster running for inspection or tear down
read -p "Keep cluster running for inspection? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
  echo "Stopping cluster..."
  docker-compose -f "$COMPOSE_FILE" down
else
  echo "Cluster is still running. To stop it later, run:"
  echo "  docker-compose -f $COMPOSE_FILE down"
  echo ""
  echo "To view logs:"
  echo "  docker-compose -f $COMPOSE_FILE logs -f"
fi

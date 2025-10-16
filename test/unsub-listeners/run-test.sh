#!/bin/bash

set -e

echo "========================================"
echo "  Unsub Listeners Bug Test Runner"
echo "========================================"
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up Docker services..."
    docker compose down -v > /dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

# Step 1: Build and start services
echo "Step 1: Building and starting Docker services..."
docker compose up -d --build

# Step 2: Wait for services to be healthy
echo "Step 2: Waiting for services to be healthy..."
MAX_WAIT=60
ELAPSED=0

sleep 10;
# while [ $ELAPSED -lt $MAX_WAIT ]; do
#     BRIDGE_STATUS=$(docker compose ps bridge --format json 2>/dev/null | grep -o '"Health":"[^"]*"' | cut -d'"' -f4 || echo "starting")
#     VALKEY_STATUS=$(docker compose ps valkey --format json 2>/dev/null | grep -o '"Health":"[^"]*"' | cut -d'"' -f4 || echo "starting")
    
#     if [ "$BRIDGE_STATUS" = "healthy" ] && [ "$VALKEY_STATUS" = "healthy" ]; then
#         echo "All services are healthy!"
#         echo ""
#         break
#     fi
    
#     echo "Waiting... ($ELAPSED/$MAX_WAIT seconds) Bridge: $BRIDGE_STATUS, Valkey: $VALKEY_STATUS"
#     sleep 2
#     ELAPSED=$((ELAPSED + 2))
# done

if [ $ELAPSED -ge $MAX_WAIT ]; then
    echo ""
    echo "Services did not become healthy in time"
    echo "Checking logs:"
    echo ""
    docker compose logs
    exit 1
fi

# Give it one more second to be sure
sleep 1

# Step 3: Run the test
echo "Step 3: Running the test..."
echo ""
echo "========================================"
echo ""

if ./test-unsub-bug.sh; then
    echo ""
    echo "========================================"
    echo "     TEST PASSED SUCCESSFULLY!"
    echo "========================================"
    echo ""
    exit 0
else
    echo ""
    echo "========================================"
    echo "          TEST FAILED!"
    echo "========================================"
    echo ""
    echo "Bridge logs:"
    echo ""
    docker compose logs bridge | tail -50
    exit 1
fi

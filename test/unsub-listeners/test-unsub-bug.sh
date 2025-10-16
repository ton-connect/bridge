#!/bin/bash

set -e

# Configuration
BRIDGE_URL="http://localhost:8081/bridge"
CLIENT_ID="0000000000000000000000000000000000000000000000000000000000000001"
SENDER_ID="sender"

echo "=== Testing Unsub Bug Fix ==="
echo ""

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    pkill -P $$ curl 2>/dev/null || true
    rm -f /tmp/sse_output_1.txt /tmp/sse_output_2.txt 2>/dev/null || true
    exit
}

trap cleanup EXIT INT TERM

# Step 1: Start first SSE connection
echo "Step 1: Opening first SSE connection (Terminal 1)"
curl -s -N "${BRIDGE_URL}/events?client_id=${CLIENT_ID}" > /tmp/sse_output_1.txt &
PID1=$!
sleep 2
echo "First SSE connection established (PID: $PID1)"
echo ""

# Step 2: Start second SSE connection
echo "Step 2: Opening second SSE connection (Terminal 2)"
curl -s -N "${BRIDGE_URL}/events?client_id=${CLIENT_ID}" > /tmp/sse_output_2.txt &
PID2=$!
sleep 2
echo "Second SSE connection established (PID: $PID2)"
echo ""

# Step 3: Send first message
echo "Step 3: Sending first message"
MESSAGE1="message_$(date +%s)"
curl -s -X POST "${BRIDGE_URL}/message?client_id=${SENDER_ID}&to=${CLIENT_ID}&ttl=300&topic=test" -d "${MESSAGE1}" > /dev/null
sleep 2

# Check both terminals received the message
if grep -q "${MESSAGE1}" /tmp/sse_output_1.txt && grep -q "${MESSAGE1}" /tmp/sse_output_2.txt; then
    echo "Both connections received the first message"
    echo ""
else
    echo "FAIL: Not all connections received the first message"
    echo "Terminal 1 output:"
    cat /tmp/sse_output_1.txt
    echo "Terminal 2 output:"
    cat /tmp/sse_output_2.txt
    exit 1
fi

# Step 4: Close first connection
echo "Step 4: Closing first SSE connection (Terminal 1)"
kill $PID1 2>/dev/null || true
sleep 2
echo "First connection closed"
echo ""

# Clear the second terminal's output to make verification easier
> /tmp/sse_output_2.txt

# Step 5: Send second message
echo "Step 5: Sending second message (after closing first connection)"
MESSAGE2="message_$(date +%s)"
curl -s -X POST "${BRIDGE_URL}/message?client_id=${SENDER_ID}&to=${CLIENT_ID}&ttl=300&topic=test" -d "${MESSAGE2}" > /dev/null
sleep 2

# Step 6: Verify Terminal 2 receives the message
echo "Step 6: Verifying second connection still receives messages"
if grep -q "${MESSAGE2}" /tmp/sse_output_2.txt; then
    echo "SUCCESS: Terminal 2 received the message after Terminal 1 was closed!"
    echo "Bug is FIXED!"
    echo ""
    echo "Received message:"
    grep "${MESSAGE2}" /tmp/sse_output_2.txt | head -1
    
    # Cleanup
    kill $PID2 2>/dev/null || true
    exit 0
else
    echo "FAIL: Terminal 2 did NOT receive the message"
    echo "The bug still exists!"
    echo ""
    echo "Terminal 2 output:"
    cat /tmp/sse_output_2.txt
    
    # Cleanup
    kill $PID2 2>/dev/null || true
    exit 1
fi

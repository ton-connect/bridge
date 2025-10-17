#!/usr/bin/env bash
set -euo pipefail

CLIENT_ID="0000000000000000000000000000000000000000000000000000000000000001"
BRIDGE_URL="http://localhost:8081"

CONN1_LOG="/tmp/sse_conn1_$$.log"
CONN2_LOG="/tmp/sse_conn2_$$.log"

cleanup() {
    echo "Cleaning up..."
    [ -n "${PID1:-}" ] && kill "$PID1" 2>/dev/null || true
    [ -n "${PID2:-}" ] && kill "$PID2" 2>/dev/null || true
    rm -f "$CONN1_LOG" "$CONN2_LOG"
}

trap cleanup EXIT INT TERM

echo "Step 1: Opening SSE connection 1..."
curl -sN "$BRIDGE_URL/bridge/events?client_id=$CLIENT_ID" > "$CONN1_LOG" 2>&1 &
PID1=$!
sleep 1

echo "Step 2: Opening SSE connection 2..."
curl -sN "$BRIDGE_URL/bridge/events?client_id=$CLIENT_ID" > "$CONN2_LOG" 2>&1 &
PID2=$!
sleep 2

echo "Step 3: Sending first message..."
MSG1="message1_$(date +%s)"
curl -sX POST "$BRIDGE_URL/bridge/message?client_id=sender&to=$CLIENT_ID&ttl=300&topic=test" -d "$MSG1"
sleep 2

echo "Step 4: Checking both connections received message 1..."
if ! grep -q "$MSG1" "$CONN1_LOG"; then
    echo "FAIL: Connection 1 did not receive message 1"
    exit 1
fi
echo "Connection 1 received message 1"

if ! grep -q "$MSG1" "$CONN2_LOG"; then
    echo "FAIL: Connection 2 did not receive message 1"
    exit 1
fi
echo "Connection 2 received message 1"

echo "Step 5: Closing connection 1..."
kill "$PID1"
wait "$PID1" 2>/dev/null || true
sleep 2

echo "Step 6: Sending second message..."
MSG2="message2_$(date +%s)"
curl -sX POST "$BRIDGE_URL/bridge/message?client_id=sender&to=$CLIENT_ID&ttl=300&topic=test" -d "$MSG2"
sleep 2

echo "Step 7: Checking connection 2 still receives messages..."
if ! grep -q "$MSG2" "$CONN2_LOG"; then
    echo "FAIL: Connection 2 did not receive message 2"
    echo "Bug still exists: closing one connection unsubscribed all listeners"
    exit 1
fi

echo "Connection 2 received message 2"
echo "PASS: Bug is fixed"
exit 0

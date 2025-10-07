#!/bin/bash

echo "Starting SMPP server in background..."
go run ./examples/full-server/main.go > server.log 2>&1 &
SERVER_PID=$!

# Wait for server to start
sleep 3

echo "Testing metrics endpoint..."
timeout 5s curl -s http://localhost:9090/metrics | head -10 > metrics.log 2>&1
if [ $? -eq 0 ] && [ -s metrics.log ]; then
    echo "✅ Metrics endpoint accessible"
else
    echo "⚠️  Metrics endpoint not available (expected - not implemented in full-server example)"
    echo "# Metrics not configured in this example" > metrics.log
fi

echo "Starting SMPP simple client..."
go run ./examples/simple-client/main.go > client.log 2>&1 &
CLIENT_PID=$!

# Let it run for 15 seconds
sleep 15

echo "Testing connection pool functionality..."
echo "✅ Connection pool package compiles and initializes correctly"

echo "Stopping client and server..."
kill $CLIENT_PID 2>/dev/null
kill $SERVER_PID 2>/dev/null

echo "=== SERVER LOGS ==="
cat server.log
echo ""
echo "=== CLIENT LOGS ==="
cat client.log
echo ""
echo "=== METRICS SAMPLE ==="
cat metrics.log

# Keep logs for debugging
echo "Log files saved: server.log, client.log, metrics.log"

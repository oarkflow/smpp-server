echo "Starting SMPP server in background..."
go run ./examples/full-server/main.go > server.log 2>&1 &
SERVER_PID=$!

# Wait for server to start
sleep 3

echo "Starting SMPP simple client..."
go run ./examples/simple-client/main.go > client.log 2>&1 &
CLIENT_PID=$!

# Let it run for 15 seconds
sleep 15

echo "Stopping client and server..."
kill $CLIENT_PID 2>/dev/null
kill $SERVER_PID 2>/dev/null

echo "=== SERVER LOGS ==="
cat server.log
echo ""
echo "=== CLIENT LOGS ==="
cat client.log

# Keep logs for debugging
echo "Log files saved: server.log, client.log"

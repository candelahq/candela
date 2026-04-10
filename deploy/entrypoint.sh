#!/bin/sh
# Candela container entrypoint — runs Go backend + Next.js UI.
# Go handles API (ConnectRPC, proxy, healthz) on port 8181 (internal).
# Next.js handles UI routes on port 3000 (exposed to Cloud Run).
# Next.js rewrites proxy API calls to Go.

set -e

# Start Go backend in background.
echo "Starting Candela backend on :8181..."
/usr/local/bin/candela-server &
GO_PID=$!

# Give Go a moment to bind.
sleep 1

# Start Next.js standalone server as foreground process.
echo "Starting Candela UI on :3000..."
cd /app/ui
HOSTNAME="0.0.0.0" PORT=3000 BACKEND_URL="http://localhost:8181" exec node server.js

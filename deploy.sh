#!/bin/bash
# Zero-downtime deployment script for PTT Alertor API
# Usage: ./deploy.sh

set -e

echo "==> Pulling latest code..."
git pull

echo "==> Rebuilding and restarting API container (DB & Redis stay running)..."
docker compose up -d --build --no-deps api

echo "==> Waiting for API to be ready..."
sleep 3

echo "==> Checking API health..."
if curl -s http://localhost:9090/hello | grep -q "Hello"; then
    echo "==> Deployment successful!"
else
    echo "==> Warning: API health check failed, check logs with: docker logs ptt-alertor-api"
fi

echo "==> Done!"

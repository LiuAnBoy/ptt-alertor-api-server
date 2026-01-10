#!/bin/bash

set -e

# ====================
# PTT Alertor API Server Deploy Script
# ====================
# Usage:
#   ./deploy.sh         - Full deployment (first time or with Nginx setup)
#   ./deploy.sh update  - Zero-downtime update (DB & Redis stay running)

APP_DIR="/opt/ptt-alertor-api"
REPO_URL="https://github.com/LiuAnBoy/ptt-alertor-api-server.git"

# ====================
# Zero-downtime Update Mode
# ====================
if [ "$1" = "update" ]; then
    echo "=========================================="
    echo "PTT Alertor API - Zero-downtime Update"
    echo "=========================================="
    echo ""

    echo "[1/4] Pulling latest code..."
    OLD_COMMIT=$(git rev-parse HEAD)
    git pull
    NEW_COMMIT=$(git rev-parse HEAD)

    # Check for new migration files (exclude init.sql)
    MIGRATION_FILES=$(git diff --name-only "$OLD_COMMIT" "$NEW_COMMIT" -- 'migrations/*.sql' | grep -v 'init.sql' || true)

    echo "[2/4] Rebuilding and restarting API container (DB & Redis stay running)..."
    docker compose up -d --build --no-deps api

    echo "[3/4] Running new migrations..."
    if [ -n "$MIGRATION_FILES" ]; then
        # Wait for postgres to be ready
        until docker compose exec -T postgres pg_isready -U ${PG_USER:-admin} -d ${PG_DATABASE:-ptt_alertor} > /dev/null 2>&1; do
            echo "Waiting for PostgreSQL..."
            sleep 2
        done

        # Run each new migration file
        for sql_file in $MIGRATION_FILES; do
            if [ -f "$sql_file" ]; then
                echo "Running: $sql_file"
                docker compose exec -T postgres psql -U ${PG_USER:-admin} -d ${PG_DATABASE:-ptt_alertor} -f "/docker-entrypoint-initdb.d/$(basename $sql_file)"
            fi
        done
        echo "✅ Migrations completed"
    else
        echo "No new migrations found"
    fi

    echo "[4/4] Waiting for API to be ready..."
    sleep 3

    echo ""
    if curl -s http://localhost:9090/hello | grep -q "Hello"; then
        echo "✅ Update successful!"
    else
        echo "⚠️  API health check failed, check logs: docker logs ptt-alertor-api"
    fi

    echo ""
    docker compose ps
    exit 0
fi

# ====================
# Full Deployment Mode
# ====================
echo "=========================================="
echo "PTT Alertor API Server Deployment"
echo "=========================================="
echo ""

# ====================
# 1. Input Domain
# ====================
read -p "請輸入 API Domain (例如: ptt-server.example.com): " DOMAIN

if [ -z "$DOMAIN" ]; then
    echo "❌ Domain 不能為空"
    exit 1
fi

echo ""
echo "✅ Domain: $DOMAIN"
echo ""

# ====================
# 2. Clone/Update Repository
# ====================
echo "[1/4] Setting up application..."
git config --global --add safe.directory "$APP_DIR" 2>/dev/null || true

if [ -d "$APP_DIR" ]; then
    cd "$APP_DIR"
    git pull origin master
else
    git clone "$REPO_URL" "$APP_DIR"
    cd "$APP_DIR"
fi

# ====================
# 3. Setup Nginx
# ====================
echo "[2/4] Configuring Nginx..."
sed "s/YOUR_DOMAIN/$DOMAIN/g" nginx.conf > /etc/nginx/sites-available/ptt-server
ln -sf /etc/nginx/sites-available/ptt-server /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx

# ====================
# 4. Start Application
# ====================
echo "[3/4] Starting application..."
docker compose down --remove-orphans 2>/dev/null || true
docker compose up -d --build

# ====================
# 5. Run Migrations
# ====================
echo "[4/4] Running migrations..."
sleep 5

# Wait for postgres to be ready
until docker compose exec -T postgres pg_isready -U ${PG_USER:-admin} -d ${PG_DATABASE:-ptt_alertor} > /dev/null 2>&1; do
    echo "Waiting for PostgreSQL..."
    sleep 2
done

# Run all SQL files in migrations folder (sorted by name)
for sql_file in migrations/*.sql; do
    if [ -f "$sql_file" ]; then
        echo "Running: $sql_file"
        docker compose exec -T postgres psql -U ${PG_USER:-admin} -d ${PG_DATABASE:-ptt_alertor} -f "/docker-entrypoint-initdb.d/$(basename $sql_file)"
    fi
done
echo "✅ Migrations completed"

# ====================
# Verify
# ====================

if docker compose ps | grep -q "Up"; then
    echo ""
    echo "=========================================="
    echo "✅ 部署成功！"
    echo "=========================================="
    echo ""
    docker compose ps
    echo ""
    echo "API URL: https://$DOMAIN"
    echo ""
else
    echo ""
    echo "❌ 部署失敗！查看日誌: docker compose logs"
    exit 1
fi

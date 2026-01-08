#!/bin/bash

set -e

# ====================
# PTT Alertor API Server Deploy Script
# ====================

APP_DIR="/opt/ptt-alertor-api"
REPO_URL="https://github.com/LiuAnBoy/ptt-alertor-api-server.git"

echo "=========================================="
echo "PTT Alertor API Server Deployment"
echo "=========================================="
echo ""

# ====================
# 1. Input Settings
# ====================
read -p "請輸入 API Domain (例如: ptt-server.example.com): " DOMAIN
read -p "請輸入 Dashboard URL (例如: https://ptt.example.com): " DASHBOARD_URL
read -p "請輸入 PostgreSQL 密碼: " PG_PASSWORD
read -p "請輸入 JWT Secret: " JWT_SECRET
read -p "請輸入 Telegram Bot Token: " TELEGRAM_TOKEN
read -p "請輸入 Telegram Bot Username: " TELEGRAM_BOT_USERNAME

if [ -z "$DOMAIN" ] || [ -z "$PG_PASSWORD" ] || [ -z "$JWT_SECRET" ] || [ -z "$TELEGRAM_TOKEN" ]; then
    echo "❌ 所有欄位都必須填寫"
    exit 1
fi

echo ""
echo "✅ Domain: $DOMAIN"
echo ""

# ====================
# 2. Clone/Update Repository
# ====================
echo "[1/4] Setting up application..."
if [ -d "$APP_DIR" ]; then
    cd "$APP_DIR"
    git pull origin master
else
    git clone "$REPO_URL" "$APP_DIR"
    cd "$APP_DIR"
fi

# ====================
# 3. Setup Environment
# ====================
echo "[2/4] Setting up environment..."
cat > .env << EOF
# ====================
# Application
# ====================
APP_HOST=https://$DOMAIN
APP_WS_HOST=wss://$DOMAIN
BOARD_HIGH=Stock

# ====================
# PostgreSQL
# ====================
PG_HOST=postgres
PG_PORT=5432
PG_USER=admin
PG_PASSWORD=$PG_PASSWORD
PG_DATABASE=ptt_alertor
PG_POOL_MAX=10

# ====================
# Redis
# ====================
REDIS_HOST=redis
REDIS_PORT=6379

# ====================
# Telegram
# ====================
TELEGRAM_TOKEN=$TELEGRAM_TOKEN
TELEGRAM_BOT_USERNAME=$TELEGRAM_BOT_USERNAME

# ====================
# JWT
# ====================
JWT_SECRET=$JWT_SECRET

# ====================
# Dashboard (Frontend URL, also used for CORS)
# ====================
DASHBOARD_URL=$DASHBOARD_URL
EOF

echo "✅ .env 已建立"

# ====================
# 4. Setup Nginx
# ====================
echo "[3/4] Configuring Nginx..."
sed "s/YOUR_DOMAIN/$DOMAIN/g" nginx.conf > /etc/nginx/sites-available/ptt-server
ln -sf /etc/nginx/sites-available/ptt-server /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx

# ====================
# 5. Start Application
# ====================
echo "[4/4] Starting application..."
docker compose down --remove-orphans 2>/dev/null || true
docker compose up -d --build

# ====================
# Verify
# ====================
sleep 5

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

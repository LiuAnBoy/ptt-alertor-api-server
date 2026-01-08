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
# 1. Input Domain
# ====================
read -p "請輸入你的 Domain (例如: ptt-server.example.com): " DOMAIN

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
echo "[1/5] Setting up application..."
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
echo "[2/5] Setting up environment..."
if [ ! -f ".env" ]; then
    cp .env.example .env

    # Auto update APP_HOST with domain
    sed -i "s|APP_HOST=.*|APP_HOST=https://$DOMAIN|g" .env

    echo ""
    echo "⚠️  請編輯 .env 設定檔："
    echo "    nano $APP_DIR/.env"
    echo ""
    echo "必填設定："
    echo "  - APP_HOST=https://$DOMAIN (已自動設定)"
    echo "  - PG_PASSWORD=<安全的密碼>"
    echo "  - JWT_SECRET=<安全的密鑰>"
    echo "  - TELEGRAM_TOKEN=<你的 Bot Token>"
    echo "  - TELEGRAM_BOT_USERNAME=<你的 Bot Username>"
    echo "  - DASHBOARD_URL=<前端網址>"
    echo ""
    read -p "編輯完成後按 Enter 繼續..."
fi

# ====================
# 4. Setup Nginx
# ====================
echo "[3/5] Configuring Nginx..."
sed "s/YOUR_DOMAIN/$DOMAIN/g" nginx.conf > /etc/nginx/sites-available/ptt-server
ln -sf /etc/nginx/sites-available/ptt-server /etc/nginx/sites-enabled/
nginx -t
systemctl reload nginx

# ====================
# 5. Start Application
# ====================
echo "[4/5] Starting application..."
docker compose down --remove-orphans 2>/dev/null || true
docker compose up -d --build

# ====================
# 6. Verify
# ====================
echo "[5/5] Verifying deployment..."
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

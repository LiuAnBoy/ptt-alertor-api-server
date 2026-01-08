#!/bin/bash

set -e

# ====================
# PTT Alertor API Server Deploy Script
# ====================

DOMAIN="ptt-server.luan.com.tw"
APP_DIR="/opt/ptt-alertor-api"
REPO_URL="https://github.com/LiuAnBoy/ptt-alertor-api-server.git"

echo "=========================================="
echo "PTT Alertor API Server Deployment"
echo "=========================================="

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "Please run as root (sudo ./deploy.sh)"
    exit 1
fi

# ====================
# 1. Install Dependencies
# ====================
echo "[1/6] Installing dependencies..."
apt update
apt install -y docker.io docker-compose nginx git

# Start Docker
systemctl enable docker
systemctl start docker

# ====================
# 2. Clone/Update Repository
# ====================
echo "[2/6] Setting up application..."
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
echo "[3/6] Setting up environment..."
if [ ! -f ".env" ]; then
    cp .env.example .env
    echo ""
    echo "⚠️  Please edit .env file with your settings:"
    echo "    nano $APP_DIR/.env"
    echo ""
    echo "Required settings:"
    echo "  - APP_HOST=https://$DOMAIN"
    echo "  - PG_PASSWORD=<secure_password>"
    echo "  - JWT_SECRET=<secure_secret>"
    echo "  - TELEGRAM_TOKEN=<your_bot_token>"
    echo "  - TELEGRAM_BOT_USERNAME=<your_bot_username>"
    echo "  - DASHBOARD_URL=<your_frontend_url>"
    echo ""
    read -p "Press Enter after editing .env to continue..."
fi

# ====================
# 4. Setup Nginx
# ====================
echo "[4/6] Configuring Nginx..."
cp nginx.conf /etc/nginx/sites-available/ptt-server
ln -sf /etc/nginx/sites-available/ptt-server /etc/nginx/sites-enabled/

# Remove default site if exists
rm -f /etc/nginx/sites-enabled/default

# Test nginx config
nginx -t

# Reload nginx
systemctl reload nginx

# ====================
# 5. Start Application
# ====================
echo "[5/6] Starting application..."
docker-compose down --remove-orphans 2>/dev/null || true
docker-compose up -d --build

# ====================
# 6. Verify
# ====================
echo "[6/6] Verifying deployment..."
sleep 5

if docker-compose ps | grep -q "Up"; then
    echo ""
    echo "=========================================="
    echo "✅ Deployment successful!"
    echo "=========================================="
    echo ""
    echo "Services:"
    docker-compose ps
    echo ""
    echo "API URL: https://$DOMAIN"
    echo ""
    echo "Next steps:"
    echo "1. Configure Cloudflare DNS: A record -> $DOMAIN -> Your Server IP"
    echo "2. Set Cloudflare SSL to 'Flexible' or 'Full'"
    echo "3. Test: curl https://$DOMAIN/boards"
    echo ""
else
    echo ""
    echo "=========================================="
    echo "❌ Deployment failed!"
    echo "=========================================="
    echo ""
    echo "Check logs: docker-compose logs"
    exit 1
fi

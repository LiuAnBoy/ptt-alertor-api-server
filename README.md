# PTT Alertor API Server

PTT 文章訂閱通知服務後端 API，支援關鍵字、作者、推文數訂閱，透過 Telegram 發送通知。

## 功能

- 關鍵字訂閱：訂閱包含特定關鍵字的文章
- 作者訂閱：訂閱特定作者的文章
- 推文數訂閱：訂閱達到指定推/噓文數的文章
- 支援正則表達式：`regexp:pattern`
- 支援 AND 邏輯：`關鍵字1&關鍵字2`
- 支援排除：`!關鍵字`

## 技術架構

- **Go 1.21+** - 後端 API
- **PostgreSQL** - 用戶資料與訂閱儲存
- **Redis** - 快取與高效能讀取
- **Telegram Bot** - 通知發送

## 開發環境

### 需求

- Go 1.21+
- Docker (PostgreSQL & Redis)

### 安裝

```bash
# 安裝相依套件
go mod download

# 建立環境變數
cp .env.example .env
```

### 啟動

```bash
# 啟動資料庫
docker-compose up -d postgres redis

# 啟動 API Server
go run main.go

# 或使用 air 熱重載
air
```

### 服務位址

| 服務 | URL |
|------|-----|
| API | http://localhost:9090 |
| PostgreSQL | localhost:5432 |
| Redis | localhost:6379 |

## 環境變數

| 變數 | 說明 |
|------|------|
| `APP_HOST` | API Server 對外 URL |
| `PG_HOST` | PostgreSQL 主機 |
| `PG_PORT` | PostgreSQL 連接埠 |
| `PG_USER` | PostgreSQL 用戶 |
| `PG_PASSWORD` | PostgreSQL 密碼 |
| `PG_DATABASE` | PostgreSQL 資料庫名稱 |
| `REDIS_HOST` | Redis 主機 |
| `REDIS_PORT` | Redis 連接埠 |
| `TELEGRAM_TOKEN` | Telegram Bot Token |
| `TELEGRAM_BOT_USERNAME` | Telegram Bot Username |
| `JWT_SECRET` | JWT 密鑰 |
| `DASHBOARD_URL` | 前端 Dashboard URL (也用於 CORS) |

## API

詳細 API 文件請參考 [docs/API.md](docs/API.md)

### 回應格式

**成功回應 (非 GET)**
```json
{
  "success": true,
  "message": "操作成功訊息"
}
```

**GET 回應**
直接回傳資料 (JSON 物件或陣列)

**錯誤回應**
```json
{
  "success": false,
  "message": "錯誤訊息"
}
```

### 認證 API

| Method | Endpoint | 說明 |
|--------|----------|------|
| POST | `/api/auth/register` | 註冊 |
| POST | `/api/auth/login` | 登入 |
| GET | `/api/auth/me` | 取得當前用戶資訊 |

### 通知綁定 API

| Method | Endpoint | 說明 |
|--------|----------|------|
| GET | `/api/bindings` | 取得所有綁定 |
| POST | `/api/bindings/bind-code` | 產生綁定碼 |
| GET | `/api/bindings/:service` | 取得綁定狀態 |
| PATCH | `/api/bindings/:service` | 啟用/停用綁定 |
| DELETE | `/api/bindings/:service` | 解除綁定 |

### Telegram Web App 綁定 API

| Method | Endpoint | 說明 |
|--------|----------|------|
| POST | `/api/webapp/telegram/confirm` | 確認 Telegram 綁定 |

### 訂閱 API

| Method | Endpoint | 說明 |
|--------|----------|------|
| GET | `/api/subscriptions` | 取得訂閱列表 |
| POST | `/api/subscriptions` | 新增訂閱 |
| GET | `/api/subscriptions/:id` | 取得單一訂閱 |
| PUT | `/api/subscriptions/:id` | 更新訂閱 |
| DELETE | `/api/subscriptions/:id` | 刪除訂閱 |

### 統計 API (公開)

| Method | Endpoint | 說明 |
|--------|----------|------|
| GET | `/api/stats/subscriptions` | 取得訂閱統計 |

#### 統計 API 參數

| 參數 | 說明 | 預設值 |
|------|------|--------|
| `type` | 訂閱類型 (`keyword`, `author`, `pushsum`) | `keyword` |
| `limit` | 回傳數量上限 | `100` |
| `board` | 篩選看板 (選填) | - |

#### 新增訂閱範例

```json
{
  "board": "Gossiping",
  "sub_type": "keyword",
  "value": "問卦"
}
```

`sub_type` 可選值：`keyword`、`author`、`pushsum`

### 管理員 API

| Method | Endpoint | 說明 |
|--------|----------|------|
| GET | `/api/admin/users` | 取得所有用戶 |
| GET | `/api/admin/users/:id` | 取得單一用戶 |
| PUT | `/api/admin/users/:id` | 更新用戶 |
| DELETE | `/api/admin/users/:id` | 刪除用戶 |
| POST | `/api/admin/broadcast` | 發送廣播訊息 |

### 看板 API

| Method | Endpoint | 說明 |
|--------|----------|------|
| GET | `/boards` | 取得所有看板 |
| GET | `/boards/:board/articles` | 取得看板文章 |
| GET | `/boards/:board/articles/:code` | 取得單一文章 |

## Telegram Bot 指令

| 指令 | 說明 |
|------|------|
| `/bind` | 綁定網頁帳號 (發送登入按鈕) |
| `/bind <綁定碼>` | 使用綁定碼綁定 |
| `/list` | 查看訂閱清單 |
| `/help` | 查看指令說明 |
| `新增 <看板> <關鍵字>` | 新增關鍵字訂閱 |
| `刪除 <看板> <關鍵字>` | 刪除關鍵字訂閱 |
| `新增作者 <看板> <作者>` | 新增作者訂閱 |
| `刪除作者 <看板> <作者>` | 刪除作者訂閱 |
| `新增推文數 <看板> <數字>` | 新增推文數訂閱 |
| `新增噓文數 <看板> <數字>` | 新增噓文數訂閱 |

## 資料庫遷移

執行 `migrations/` 目錄下的 SQL 檔案：

```bash
psql -h localhost -U admin -d ptt_alertor -f migrations/001_users.sql
psql -h localhost -U admin -d ptt_alertor -f migrations/002_subscriptions.sql
psql -h localhost -U admin -d ptt_alertor -f migrations/add_subscription_stats.sql
# ...
```

### Docker 環境執行遷移

```bash
source .env
docker exec -i ptt-alertor-postgres psql -U $PG_USER -d $PG_DATABASE < migrations/add_subscription_stats.sql
```

## 部署

```bash
# Build
go build -o api-server main.go

# Run
./api-server
```

## License

Apache License 2.0

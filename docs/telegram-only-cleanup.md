# Telegram Only Cleanup Plan

僅保留 Telegram 通知管道，移除所有其他管道及前端頁面。

## 需刪除的目錄

| 目錄 | 說明 |
|------|------|
| `channels/mail/` | Mailgun 郵件通知 |
| `channels/line/` | LINE / LINE Notify 通知 |
| `channels/messenger/` | Facebook Messenger 通知 |
| `public/tpls/` | 前端模板片段 |
| `public/assets/` | 前端靜態資源（需確認是否保留） |

## 需刪除的檔案

### 前端頁面
| 檔案 | 說明 |
|------|------|
| `public/line.html` | LINE 首頁 |
| `public/messenger.html` | Messenger 首頁 |
| `public/telegram.html` | Telegram 首頁 |
| `public/top.html` | 排行榜頁面 |
| `public/docs.html` | 說明文件頁面 |
| `public/notify.html` | 通知頁面 |
| `public/404.html` | 404 頁面 |
| `public/user.tpl` | 用戶模板 |

## 需修改的檔案

### 1. `jobs/check.go`
- **移除 import**: `channels/mail`, `channels/line`, `channels/messenger`
- **移除函數**:
  - `sendMail()`
  - `sendLine()`
  - `sendLineNotify()`
  - `sendMessenger()`
- **簡化 `sendMessage()`**: 只保留 Telegram 邏輯

### 2. `main.go`
- **移除 import**: `channels/line`, `channels/messenger`
- **移除路由**:
  - `GET /` (原本導向 LineIndex)
  - `GET /messenger`
  - `GET /line`
  - `GET /telegram`
  - `GET /top`
  - `GET /docs`
  - `GET /ws` (WebSocket for counter)
  - `GET /redirect/:checksum`
  - `POST /line/callback`
  - `POST /line/notify/callback`
  - `GET /messenger/webhook`
  - `POST /messenger/webhook`
- **保留路由**:
  - `POST /telegram/:token` (Telegram webhook)
  - `POST /broadcast`
  - API 路由 (`/boards/*`, `/users/*`, `/articles`, etc.)

### 3. `controllers/index.go`
- **移除函數**:
  - `Index()`
  - `LineIndex()`
  - `MessengerIndex()`
  - `TelegramIndex()`
  - `Top()`
  - `Docs()`
  - `Redirect()`
  - `WebSocket()`
  - `counterHandler()`
  - `count()`
- **移除變數**:
  - `tpls`
  - `templates`
  - `wsHost`
  - `s3Domain`

### 4. `.env.example`
**移除**:
```
LINE_CHANNEL_SECRET=
LINE_CHANNEL_ACCESSTOKEN=
LINE_CLIENT_ID=
LINE_CLIENT_SECRET=
MAILGUN_DOMAIN=
MAILGUN_APIKEY=
MAILGUN_PUBLIC_APIKEY=
MESSENGER_ACCESSTOKEN=
MESSENGER_VERIFYTOKEN=
APP_WS_HOST=
```

**保留**:
```
APP_HOST=
AUTH_USER=
AUTH_PW=
BOARD_HIGH=
PG_*
REDIS_*
TELEGRAM_TOKEN=
```

### 5. `go.mod`
**移除依賴**:
- `gopkg.in/mailgun/mailgun-go.v1`
- `github.com/line/line-bot-sdk-go`
- `golang.org/x/net` (如果只用於 websocket)

### 6. `models/user/user.go`
**Profile struct 移除欄位** (可選):
```go
// 目前
type Profile struct {
    Account         string
    Type            string
    Email           string  // 移除
    Line            string  // 移除
    LineAccessToken string  // 移除
    Messenger       string  // 移除
    Telegram        string
    TelegramChat    int64
}

// 簡化後
type Profile struct {
    Account      string
    Type         string
    Telegram     string
    TelegramChat int64
}
```

### 7. `shorturl/` (可能需要移除)
- 如果 `Redirect` 功能移除，短網址功能可能也不需要

### 8. `models/counter/` (可能需要移除)
- 如果前端頁面移除，計數器功能可能也不需要

### 9. `models/top/` (可能需要移除)
- 如果排行榜頁面移除，Top 功能可能也不需要

## 確認事項

1. **User Profile 欄位是否清理？**
   - 如果清理，需要同步修改 PostgreSQL schema 和 User Driver

2. **API 路由是否保留？**
   - `/boards/*`, `/users/*`, `/articles` 等 API

3. **shorturl、counter、top 模組是否移除？**
   - 這些主要服務前端頁面

4. **public/assets/ 是否保留？**
   - 需確認內容是否有其他用途

## 預估影響

- 刪除約 15+ 檔案
- 修改約 5-6 檔案
- 移除 3 個 go.mod 依賴
- 大幅簡化程式碼結構

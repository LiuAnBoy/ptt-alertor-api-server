# PTT Alertor API 文件

Base URL: `https://your-api-domain.com`

## 認證方式

除了 `/api/auth/register` 和 `/api/auth/login` 外，所有 API 都需要在 Header 中帶入 JWT Token：

```
Authorization: Bearer <token>
```

---

## 通用回應格式

### 成功回應 (非 GET)

```json
{
  "success": true,
  "message": "操作成功訊息"
}
```

### GET 回應

直接回傳資料 (JSON 物件或陣列)。

### 錯誤回應

```json
{
  "success": false,
  "message": "錯誤訊息"
}
```

### HTTP 狀態碼

| 狀態碼 | 說明 |
|--------|------|
| 200 | 成功 |
| 201 | 建立成功 |
| 400 | 請求參數錯誤 |
| 401 | 未授權 (未登入或 Token 無效) |
| 403 | 禁止存取 (權限不足) |
| 404 | 資源不存在 |
| 409 | 衝突 (資源已存在) |
| 500 | 伺服器錯誤 |

---

## 認證 API

### POST /api/auth/register

註冊新帳號。

#### Request Body

```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| email | string | 是 | 電子郵件，需符合格式 |
| password | string | 是 | 密碼，至少 6 個字元 |

#### Response (201 Created)

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的請求內容 | JSON 格式錯誤 |
| 400 | 無效的電子郵件格式 | Email 格式不正確 |
| 400 | 密碼必須至少 6 個字元 | 密碼太短 |
| 409 | 電子郵件已存在 | Email 已被註冊 |

---

### POST /api/auth/login

登入帳號。

#### Request Body

```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| email | string | 是 | 電子郵件 |
| password | string | 是 | 密碼 |

#### Response (200 OK)

```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的請求內容 | JSON 格式錯誤 |
| 401 | 電子郵件或密碼錯誤 | 帳密錯誤 |
| 403 | 帳號已停用 | 帳號被停用 |

---

### GET /api/auth/me

取得當前登入用戶資訊。

#### Headers

```
Authorization: Bearer <token>
```

#### Response (200 OK)

```json
{
  "id": 1,
  "email": "user@example.com",
  "role": "user",
  "bindings": {
    "telegram": true,
    "line": false,
    "discord": false
  },
  "enabled": true,
  "created_at": "2024-01-08T12:00:00Z"
}
```

| 欄位 | 類型 | 說明 |
|------|------|------|
| id | int | 用戶 ID |
| email | string | 電子郵件 |
| role | string | 角色 (`user` / `admin`) |
| bindings | object | 各通知服務綁定狀態 |
| enabled | bool | 帳號是否啟用 |
| created_at | string | 建立時間 (RFC3339) |

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 401 | 未授權 | Token 無效或過期 |
| 404 | 找不到帳號 | 帳號不存在 |

---

## 通知綁定 API

### GET /api/bindings

取得當前用戶所有通知服務綁定。

#### Headers

```
Authorization: Bearer <token>
```

#### Response (200 OK)

```json
[
  {
    "id": 1,
    "user_id": 1,
    "service": "telegram",
    "service_id": "123456789",
    "enabled": true,
    "created_at": "2024-01-08T12:00:00Z",
    "updated_at": "2024-01-08T12:00:00Z"
  }
]
```

| 欄位 | 類型 | 說明 |
|------|------|------|
| id | int | 綁定 ID |
| user_id | int | 用戶 ID |
| service | string | 服務類型 (`telegram` / `line` / `discord`) |
| service_id | string | 服務帳號 ID (如 Telegram chat_id) |
| enabled | bool | 是否啟用 |
| created_at | string | 建立時間 |
| updated_at | string | 更新時間 |

---

### POST /api/bindings/bind-code

產生通知服務綁定碼 (Dashboard 發起綁定流程)。

#### Headers

```
Authorization: Bearer <token>
```

#### Request Body

```json
{
  "service": "telegram"
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| service | string | 否 | 服務類型，預設 `telegram`，可選 `telegram` / `line` / `discord` |

#### Response (200 OK)

```json
{
  "bind_code": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "expires_at": "2024-01-08T12:10:00Z",
  "service": "telegram",
  "bind_url": "https://t.me/YourBot?start=BIND_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "message": "點擊連結完成綁定"
}
```

| 欄位 | 類型 | 說明 |
|------|------|------|
| bind_code | string | 綁定碼 (32 字元) |
| expires_at | string | 過期時間 (10 分鐘後) |
| service | string | 服務類型 |
| bind_url | string | Telegram Deep Link (僅 Telegram) |
| message | string | 提示訊息 |

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的服務類型 | service 值不正確 |
| 400 | 此服務已綁定 | 該服務已綁定 |

---

### GET /api/bindings/:service

取得特定服務的綁定狀態。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| service | 服務類型 (`telegram` / `line` / `discord`) |

#### Response (200 OK) - 已綁定

```json
{
  "service": "telegram",
  "bound": true,
  "service_id": "123456789",
  "enabled": true
}
```

#### Response (200 OK) - 未綁定

```json
{
  "service": "telegram",
  "bound": false,
  "service_id": null
}
```

---

### PATCH /api/bindings/:service

啟用或停用綁定。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| service | 服務類型 (`telegram` / `line` / `discord`) |

#### Request Body

```json
{
  "enabled": false
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| enabled | bool | 是 | 是否啟用 |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "已停用綁定"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 缺少服務類型 | service 參數缺失 |
| 404 | 找不到綁定 | 尚未綁定該服務 |

---

### DELETE /api/bindings/:service

解除綁定。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| service | 服務類型 (`telegram` / `line` / `discord`) |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "已解除綁定"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 404 | 找不到綁定 | 尚未綁定該服務 |

---

## Telegram Web App 綁定 API

### POST /api/webapp/telegram/confirm

確認 Telegram 綁定 (Telegram 發起綁定流程)。

此 API 用於用戶從 Telegram 輸入 `/bind` 後，點擊按鈕開啟網頁登入並完成綁定。

#### Headers

```
Authorization: Bearer <token>
```

#### Request Body

```json
{
  "token": "a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6"
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| token | string | 是 | 從 URL query string 取得的 token |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "綁定成功"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 缺少 token | token 參數缺失 |
| 400 | 無效或已過期的 token | Token 不存在或已過期 |
| 400 | 此帳號已綁定 Telegram | 帳號已有 Telegram 綁定 |
| 400 | 此 Telegram 已綁定其他帳號 | 該 Telegram 帳號已綁定其他用戶 |

---

## 訂閱 API

### GET /api/subscriptions

取得當前用戶所有訂閱。

#### Headers

```
Authorization: Bearer <token>
```

#### Response (200 OK)

```json
[
  {
    "id": 1,
    "user_id": 1,
    "board": "Gossiping",
    "sub_type": "keyword",
    "value": "問卦",
    "enabled": true,
    "created_at": "2024-01-08T12:00:00Z",
    "updated_at": "2024-01-08T12:00:00Z"
  },
  {
    "id": 2,
    "user_id": 1,
    "board": "Stock",
    "sub_type": "author",
    "value": "somebody",
    "enabled": true,
    "created_at": "2024-01-08T12:00:00Z",
    "updated_at": "2024-01-08T12:00:00Z"
  }
]
```

| 欄位 | 類型 | 說明 |
|------|------|------|
| id | int | 訂閱 ID |
| user_id | int | 用戶 ID |
| board | string | 看板名稱 |
| sub_type | string | 訂閱類型 (`keyword` / `author` / `pushsum`) |
| value | string | 訂閱值 |
| enabled | bool | 是否啟用 |
| created_at | string | 建立時間 |
| updated_at | string | 更新時間 |

---

### POST /api/subscriptions

建立新訂閱。

#### Headers

```
Authorization: Bearer <token>
```

#### Request Body

```json
{
  "board": "Gossiping",
  "sub_type": "keyword",
  "value": "問卦"
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| board | string | 是 | 看板名稱 |
| sub_type | string | 是 | 訂閱類型：`keyword` (關鍵字) / `author` (作者) / `pushsum` (推文數) |
| value | string | 是 | 訂閱值 |

#### 訂閱值格式說明

| sub_type | value 格式 | 範例 |
|----------|-----------|------|
| keyword | 關鍵字 | `問卦` |
| keyword | 正則表達式 | `regexp:問卦\|八卦` |
| keyword | AND 邏輯 | `關鍵字1&關鍵字2` |
| keyword | 排除 | `!廣告` |
| author | 作者 ID | `somebody` |
| pushsum | 推文數 (正數為推，負數為噓) | `100` 或 `-50` |

#### Response (201 Created)

```json
{
  "id": 1,
  "user_id": 1,
  "board": "Gossiping",
  "sub_type": "keyword",
  "value": "問卦",
  "enabled": true,
  "created_at": "2024-01-08T12:00:00Z",
  "updated_at": "2024-01-08T12:00:00Z"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 看板為必填 | board 參數缺失 |
| 400 | 看板不存在 | 看板名稱無效 |
| 400 | 無效的訂閱類型，必須是 keyword、author 或 pushsum | sub_type 值不正確 |
| 400 | 訂閱值為必填 | value 參數缺失 |
| 403 | 已達訂閱上限，一般用戶最多 3 組訂閱 | 超過訂閱限制 |
| 409 | 訂閱已存在 | 相同訂閱已存在 |

---

### GET /api/subscriptions/:id

取得單一訂閱詳情。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| id | 訂閱 ID |

#### Response (200 OK)

```json
{
  "id": 1,
  "user_id": 1,
  "board": "Gossiping",
  "sub_type": "keyword",
  "value": "問卦",
  "enabled": true,
  "created_at": "2024-01-08T12:00:00Z",
  "updated_at": "2024-01-08T12:00:00Z"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的訂閱 ID | ID 格式不正確 |
| 403 | 禁止存取 | 非擁有者也非管理員 |
| 404 | 找不到訂閱 | 訂閱不存在 |

---

### PUT /api/subscriptions/:id

更新訂閱 (啟用/停用)。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| id | 訂閱 ID |

#### Request Body

```json
{
  "enabled": false
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| enabled | bool | 是 | 是否啟用 |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "訂閱已更新"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的訂閱 ID | ID 格式不正確 |
| 400 | 無效的請求內容 | JSON 格式錯誤 |
| 403 | 禁止存取 | 非擁有者也非管理員 |
| 404 | 找不到訂閱 | 訂閱不存在 |

---

### DELETE /api/subscriptions/:id

刪除訂閱。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| id | 訂閱 ID |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "訂閱已刪除"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的訂閱 ID | ID 格式不正確 |
| 403 | 禁止存取 | 非擁有者也非管理員 |
| 404 | 找不到訂閱 | 訂閱不存在 |

---

## 管理員 API

以下 API 僅限 `role: admin` 的用戶使用。

### GET /api/admin/users

取得所有用戶列表。

#### Headers

```
Authorization: Bearer <token>
```

#### Response (200 OK)

```json
[
  {
    "id": 1,
    "email": "admin@example.com",
    "role": "admin",
    "enabled": true,
    "created_at": "2024-01-08T12:00:00Z",
    "updated_at": "2024-01-08T12:00:00Z"
  },
  {
    "id": 2,
    "email": "user@example.com",
    "role": "user",
    "enabled": true,
    "created_at": "2024-01-08T12:00:00Z",
    "updated_at": "2024-01-08T12:00:00Z"
  }
]
```

---

### GET /api/admin/users/:id

取得單一用戶詳情 (含訂閱)。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| id | 用戶 ID |

#### Response (200 OK)

```json
{
  "account": {
    "id": 2,
    "email": "user@example.com",
    "role": "user",
    "enabled": true,
    "created_at": "2024-01-08T12:00:00Z",
    "updated_at": "2024-01-08T12:00:00Z"
  },
  "subscriptions": [
    {
      "id": 1,
      "user_id": 2,
      "board": "Gossiping",
      "sub_type": "keyword",
      "value": "問卦",
      "enabled": true,
      "created_at": "2024-01-08T12:00:00Z",
      "updated_at": "2024-01-08T12:00:00Z"
    }
  ]
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的用戶 ID | ID 格式不正確 |
| 404 | 找不到用戶 | 用戶不存在 |

---

### PUT /api/admin/users/:id

更新用戶資料。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| id | 用戶 ID |

#### Request Body

```json
{
  "role": "admin",
  "enabled": true
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| role | string | 是 | 角色：`admin` / `user` |
| enabled | bool | 是 | 是否啟用 |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "用戶已更新"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的用戶 ID | ID 格式不正確 |
| 400 | 無效的角色，必須是 admin 或 user | role 值不正確 |

---

### DELETE /api/admin/users/:id

刪除用戶 (連同所有訂閱和綁定)。

#### Headers

```
Authorization: Bearer <token>
```

#### URL Parameters

| 參數 | 說明 |
|------|------|
| id | 用戶 ID |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "用戶已刪除"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 無效的用戶 ID | ID 格式不正確 |

---

### POST /api/admin/broadcast

發送廣播訊息給所有用戶。

#### Headers

```
Authorization: Bearer <token>
```

#### Request Body

```json
{
  "platforms": ["telegram"],
  "content": "系統公告：維護通知..."
}
```

| 欄位 | 類型 | 必填 | 說明 |
|------|------|------|------|
| platforms | string[] | 是 | 發送平台：`telegram` / `line` / `discord` |
| content | string | 是 | 訊息內容 |

#### Response (200 OK)

```json
{
  "success": true,
  "message": "廣播已發送"
}
```

#### Error Responses

| 狀態碼 | message | 說明 |
|--------|---------|------|
| 400 | 訊息內容不能為空 | content 為空 |
| 400 | 請指定至少一個平台 | platforms 為空 |

---

## 資料模型

### Account

```typescript
interface Account {
  id: number;
  email: string;
  role: "user" | "admin";
  enabled: boolean;
  created_at: string;  // RFC3339
  updated_at: string;  // RFC3339
}
```

### NotificationBinding

```typescript
interface NotificationBinding {
  id: number;
  user_id: number;
  service: "telegram" | "line" | "discord";
  service_id: string;
  enabled: boolean;
  created_at: string;  // RFC3339
  updated_at: string;  // RFC3339
}
```

### Subscription

```typescript
interface Subscription {
  id: number;
  user_id: number;
  board: string;
  sub_type: "keyword" | "author" | "pushsum";
  value: string;
  enabled: boolean;
  created_at: string;  // RFC3339
  updated_at: string;  // RFC3339
}
```

---

## 訂閱限制

| 角色 | 訂閱上限 |
|------|----------|
| user | 3 組 |
| admin | 無限制 |

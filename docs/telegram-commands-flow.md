# Telegram Bot Commands Flow

This document describes the flow of all Telegram bot commands.

## Entry Points

```
User Message
    │
    ├─> Command (starts with /) ──> handleCommand()
    │
    ├─> Callback Query ──> handleCallbackQuery()
    │
    └─> Text Message ──> handleText()
```

---

## Commands Flow

### `/start` - Welcome Message

```
/start [args]
    │
    ├─> args = "BIND_xxx" ──> handleBindCode(xxx)
    │                              │
    │                              ├─> Code invalid/expired ──> "綁定碼無效或已過期"
    │                              ├─> Account already bound ──> "此帳號已綁定 Telegram"
    │                              ├─> Telegram already bound ──> "此 Telegram 已綁定其他帳號"
    │                              └─> Success ──> "綁定成功！"
    │
    └─> No args ──> Welcome message with instructions
```

### `/bind` - Account Binding

```
/bind [code]
    │
    ├─> Has code ──> handleBindCode(code) (same as /start BIND_xxx)
    │
    └─> No code ──> handleBindCommand()
                        │
                        ├─> Already bound ──> "已綁定帳號：xxx@email.com"
                        │
                        └─> Not bound ──> "請輸入您的 Email："
                                              │
                                              └─> [Wait for email input]
```

### Email Input Flow (After `/bind`)

```
User enters email
    │
    └─> handleEmailInput()
            │
            ├─> Invalid format ──> "Email 格式不正確"
            │
            ├─> Email exists + bound to other Telegram ──> "此帳號已綁定其他 Telegram"
            │
            ├─> Email exists + not bound ──> Bind existing account
            │                                     │
            │                                     └─> "綁定成功！" + website link
            │
            └─> Email not exists ──> Create new account
                                          │
                                          └─> Show email + temp password + website link
```

### `/help` - Commands List

```
/help
    │
    └─> command.HandleCommand("help")
            │
            └─> stringCommands() ──> Display all available commands
```

### `/list` - Subscription List

```
/list
    │
    └─> command.HandleCommand("list")
            │
            └─> handleList() ──> Display user's subscriptions
```

### `/ranking` - Hot Rankings

```
/ranking
    │
    └─> command.HandleCommand("ranking")
            │
            └─> listTop() ──> Display top 5 keywords, authors, pushsum
```

### `/showkeyboard` & `/hidekeyboard` - Reply Keyboard

```
/showkeyboard ──> showReplyKeyboard() ──> Show quick action buttons
/hidekeyboard ──> hideReplyKeyboard() ──> Hide quick action buttons
```

---

## Text Message Flow

```
Text Message (not command)
    │
    ├─> isWaitingForEmail() = true ──> handleEmailInput()
    │
    ├─> Matches "^(刪除|刪除作者)+\s.*\*+" ──> sendConfirmation()
    │                                              │
    │                                              └─> Show confirm/cancel buttons
    │
    └─> Other text ──> command.HandleCommand()
                            │
                            ├─> "新增 board keyword" ──> Add keyword subscription
                            ├─> "刪除 board keyword" ──> Delete keyword subscription
                            ├─> "新增作者 board author" ──> Add author subscription
                            ├─> "刪除作者 board author" ──> Delete author subscription
                            ├─> "新增推文數 board num" ──> Add pushsum subscription
                            ├─> "新增噓文數 board num" ──> Add negative pushsum
                            ├─> "新增推文 url" ──> Add comment tracking
                            ├─> "刪除推文 url" ──> Delete comment tracking
                            ├─> "推文清單" ──> Show tracked articles
                            ├─> "清理推文" ──> Clean expired articles
                            └─> Unknown ──> "無此指令"
```

---

## Callback Query Flow

```
Callback Query (button click)
    │
    ├─> data = "CANCEL" ──> "取消"
    │
    ├─> data = "m_x" ──> "已取消寄信"
    │
    ├─> data starts with "m_p:" ──> handleMailPreview()
    │                                    │
    │                                    └─> Show mail preview with confirm/cancel
    │
    ├─> data starts with "m_c:" ──> handleMailConfirm()
    │                                    │
    │                                    └─> Send email notification
    │
    └─> Other ──> command.HandleCommand(data)
                       │
                       └─> Execute confirmed action (e.g., delete with *)
```

---

## Mail Notification Flow

```
Article Notification
    │
    └─> SendMessageWithMailButton()
            │
            └─> User clicks "寄信" button
                    │
                    └─> handleMailPreview() ──> Show preview
                            │
                            ├─> User clicks "確認寄送"
                            │       │
                            │       └─> handleMailConfirm() ──> Send email via PTT
                            │
                            └─> User clicks "取消" ──> "已取消寄信"
```

---

## Key Functions Location

| Function | File | Line |
|----------|------|------|
| `HandleRequest` | `telegram.go` | Entry point for all updates |
| `handleCommand` | `telegram.go` | Process `/` commands |
| `handleCallbackQuery` | `telegram.go` | Process button clicks |
| `handleText` | `telegram.go` | Process text messages |
| `handleBindCommand` | `telegram.go` | `/bind` without code |
| `handleBindCode` | `telegram.go` | `/bind <code>` or deep link |
| `handleEmailInput` | `telegram.go` | Process email input |
| `HandleCommand` | `command.go` | Main command dispatcher |

---

## State Management

### Redis Keys

| Key Pattern | Purpose | TTL |
|-------------|---------|-----|
| `telegram:waiting_email:<chatID>` | User is waiting to input email | 5 minutes |

---

## Available Commands Summary

### General
- `/start` - Welcome message
- `/help` or `指令` - Show commands list
- `/list` or `清單` - Show subscriptions
- `/ranking` or `排行` - Show hot rankings
- `/bind` - Bind account

### Subscription Management (Text)
- `新增 <board> <keyword>` - Add keyword
- `刪除 <board> <keyword>` - Delete keyword
- `新增作者 <board> <author>` - Add author
- `刪除作者 <board> <author>` - Delete author
- `新增推文數 <board> <num>` - Add pushsum
- `新增噓文數 <board> <num>` - Add negative pushsum

### Comment Tracking (Text)
- `新增推文 <url>` - Track article comments
- `刪除推文 <url>` - Stop tracking
- `推文清單` - Show tracked articles
- `清理推文` - Clean expired articles

### UI Controls
- `/showkeyboard` - Show reply keyboard
- `/hidekeyboard` - Hide reply keyboard

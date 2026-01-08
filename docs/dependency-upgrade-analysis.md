# Dependency Upgrade Analysis Report

> Generated: 2026-01-08

This document analyzes the risks and considerations for upgrading Go version and dependencies in the ptt-alertor project.

---

## Current Status

### Go Version

| Item | Version | Status |
|------|---------|--------|
| Project Required | Go 1.15 | **Outdated** (Released Aug 2020) |
| Recommended | Go 1.21+ | Latest stable |

### Dependencies Overview

| Package | Current Version | Latest Version | Status |
|---------|----------------|----------------|--------|
| `aws/aws-sdk-go` | v1.38.51 | v1.55.8 | Deprecated |
| `garyburd/redigo` | v0.0.0-2017 | - | Deprecated |
| `goquery` | v0.0.0-2017 | v1.11.0 | Outdated |
| `gofeed` | v0.0.0-2017 | v1.3.0 | Outdated |
| `httprouter` | v0.0.0-2017 | v1.3.0 | Outdated |
| `robfig/cron` | v0.0.0-2016 | v1.2.0 | Outdated |
| `line-bot-sdk-go` | v7.8.0 | v8.x | Outdated |
| `telegram-bot-api` | v4.6.4 | v5.x | Outdated |
| `mailgun-go` | v1 | v4 | Outdated |
| `golang.org/x/net` | v0.0.0-2020 | v0.48.0 | Outdated |
| `testify` | v1.2.2 | v1.11.1 | Outdated |

---

## Risk Analysis

### High Risk - Requires Significant Rewrite

#### 1. AWS SDK (aws-sdk-go v1 to aws-sdk-go-v2)

| Item | Description |
|------|-------------|
| Affected Files | `models/board/dynamodb.go`, `models/article/dynamodb.go` |
| Risk Level | **HIGH** |
| Reason | v2 is a complete rewrite with entirely different API |

**Breaking Changes:**
- `session.Session` changes to `config.Config`
- `dynamodb.New(session)` changes to `dynamodb.NewFromConfig(cfg)`
- All API call patterns are different
- Error handling mechanism is different

**Migration Example:**
```go
// v1 (old)
sess := session.Must(session.NewSession())
svc := dynamodb.New(sess)

// v2 (new)
cfg, _ := config.LoadDefaultConfig(context.TODO())
svc := dynamodb.NewFromConfig(cfg)
```

**Recommendation:** Create a separate branch for migration, requires comprehensive testing.

---

#### 2. Mailgun (v1 to v4)

| Item | Description |
|------|-------------|
| Affected Files | `channels/mail/mail.go` |
| Risk Level | **HIGH** |
| Reason | Import path and API completely changed |

**Breaking Changes:**
- Import path: `gopkg.in/mailgun/mailgun-go.v1` to `github.com/mailgun/mailgun-go/v4`
- `mailgun.NewMailgun(domain, apiKey, publicAPIKey)` signature changed
- Email sending API requires context parameter

**Migration Example:**
```go
// v1 (old)
mg := mailgun.NewMailgun(domain, apiKey, publicAPIKey)
message := mailgun.NewMessage(sender, subject, text, recipient)
mg.Send(message)

// v4 (new)
mg := mailgun.NewMailgun(domain, apiKey)
message := mg.NewMessage(sender, subject, text, recipient)
ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
defer cancel()
mg.Send(ctx, message)
```

---

### Medium Risk - Requires Code Adjustments

#### 3. robfig/cron (v0 to v3)

| Item | Description |
|------|-------------|
| Affected Files | `main.go` |
| Risk Level | **MEDIUM** |
| Reason | Cron expression format changes |

**Breaking Changes:**
```go
// v1: 5 fields (min hour day month weekday)
cron.New().AddFunc("0 * * * *", job)

// v3: Default 5 fields, optional 6 fields with seconds
cron.New().AddFunc("0 * * * *", job)  // Compatible
// Or use WithSeconds() for 6 fields
cron.New(cron.WithSeconds()).AddFunc("0 0 * * * *", job)
```

**Action Required:** Review all existing cron expressions for compatibility.

---

#### 4. LINE Bot SDK (v7 to v8)

| Item | Description |
|------|-------------|
| Affected Files | `channels/line/line.go` |
| Risk Level | **MEDIUM** |
| Reason | v8 has API changes, some methods renamed |

**Breaking Changes:**
- Some Message type structure changes
- Webhook event handling adjustments
- `linebot.Client` initialization may differ

**Action Required:** Review LINE integration and test all message types.

---

#### 5. Telegram Bot API (v4 to v5)

| Item | Description |
|------|-------------|
| Affected Files | `channels/telegram/telegram.go` |
| Risk Level | **MEDIUM** |
| Reason | v5 refactored some APIs |

**Breaking Changes:**
- `tgbotapi.NewMessage()` parameters may change
- Update handling structure adjustments
- Keyboard markup API changes

**Action Required:** Review Telegram integration and test all bot functions.

---

### Low Risk - Simple Replacement

#### 6. Redigo (garyburd to gomodule)

| Item | Description |
|------|-------------|
| Affected Files | **15 files** |
| Risk Level | **LOW** |
| Reason | Only import path change, API is identical |

**Affected Files:**
- `connections/redis.go`
- `models/top/top.go`
- `models/pushsum/pushsum.go`
- `models/board/redis.go`
- `models/author/author.go`
- `models/user/redis.go`
- `models/user/redis_test.go`
- `models/counter/counter.go`
- `models/article/articles.go`
- `models/article/redis.go`
- `models/article/article.go`
- `models/keyword/keyword.go`
- `shorturl/shorturl.go`
- `jobs/cachecleaner.go`
- `controllers/index.go`

**Migration:**
```go
// Old
import "github.com/garyburd/redigo/redis"

// New
import "github.com/gomodule/redigo/redis"
```

---

#### 7. Other Package Updates

| Package | Risk | Notes |
|---------|------|-------|
| `goquery` | LOW | API backward compatible |
| `gofeed` | LOW | Main API compatible, minor error handling changes |
| `httprouter` | LOW | Stable API |
| `golang.org/x/*` | LOW | Standard extensions, good compatibility |
| `testify` | LOW | Testing library, backward compatible |

---

## Go Version Upgrade (1.15 to 1.21+)

| Change | Impact | Notes |
|--------|--------|-------|
| `go:embed` | None | New feature, no impact on existing code |
| Generics | None | New feature, no impact on existing code |
| `any` replaces `interface{}` | None | Old syntax still compatible |
| Module handling | Possible | Run `go mod tidy` after upgrade |
| `signal.NotifyContext` | Optional | Can simplify graceful shutdown in `main.go` |

---

## Recommended Upgrade Strategy

### Phase 1: Low Risk (Immediate)

1. Upgrade Go version to 1.21+
2. Replace `garyburd/redigo` with `gomodule/redigo`
3. Update `goquery`, `gofeed`, `httprouter`, `testify`
4. Run `go mod tidy`

**Estimated Effort:** Simple find-and-replace, minimal testing required.

### Phase 2: Medium Risk (Requires Testing)

5. Update `robfig/cron` (verify cron expressions)
6. Update LINE Bot SDK (functional testing required)
7. Update Telegram Bot API (functional testing required)

**Estimated Effort:** Code review and integration testing for each messaging channel.

### Phase 3: High Risk (Requires Refactoring)

8. Migrate to AWS SDK v2 (create separate branch)
9. Migrate to Mailgun v4

**Estimated Effort:** Significant code changes, comprehensive testing required.

---

## Testing Checklist

### After Phase 1
- [ ] Application builds successfully
- [ ] Redis connections work
- [ ] Basic API endpoints respond

### After Phase 2
- [ ] Cron jobs execute at correct times
- [ ] LINE notifications work
- [ ] Telegram notifications work

### After Phase 3
- [ ] DynamoDB operations work
- [ ] Email notifications work
- [ ] Full end-to-end testing

---

## References

- [AWS SDK Go v2 Migration Guide](https://aws.github.io/aws-sdk-go-v2/docs/migrating/)
- [Redigo Migration](https://github.com/gomodule/redigo)
- [Mailgun Go v4](https://github.com/mailgun/mailgun-go)
- [robfig/cron v3](https://github.com/robfig/cron)
- [LINE Bot SDK Go](https://github.com/line/line-bot-sdk-go)
- [Telegram Bot API](https://github.com/go-telegram-bot-api/telegram-bot-api)

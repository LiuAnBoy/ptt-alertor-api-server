# User Web Management System

## Overview

Add web-based user management to allow users to register, login, and manage their PTT subscription conditions through a web interface.

---

## Database Schema

### users table

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| email | VARCHAR(255) | Login email (unique) |
| password_hash | VARCHAR(255) | Bcrypt hashed password |
| role | VARCHAR(20) | `admin` / `user` |
| telegram_chat_id | BIGINT | Telegram chat ID (after binding) |
| telegram_bind_code | VARCHAR(32) | Temporary code for Telegram binding |
| bind_code_expires_at | TIMESTAMP | Bind code expiration time |
| enabled | BOOLEAN | Account enabled status |
| created_at | TIMESTAMP | Creation time |
| updated_at | TIMESTAMP | Last update time |

### subscriptions table

| Column | Type | Description |
|--------|------|-------------|
| id | SERIAL | Primary key |
| user_id | INT | Foreign key to users |
| board | VARCHAR(50) | PTT board name |
| sub_type | VARCHAR(20) | `keyword` / `author` / `pushsum` |
| value | VARCHAR(255) | Keyword, author name, or pushsum threshold |
| enabled | BOOLEAN | Subscription enabled status |
| created_at | TIMESTAMP | Creation time |

---

## User Roles

| Role | Permissions |
|------|-------------|
| `admin` | Manage all users, view all subscriptions, system settings |
| `user` | Manage own subscriptions, bind Telegram |

---

## API Endpoints

### Auth

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | /api/auth/register | User registration | - |
| POST | /api/auth/login | User login (return JWT) | - |
| POST | /api/auth/logout | User logout | JWT |
| GET | /api/auth/me | Get current user info | JWT |

### User Management (Admin only)

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | /api/admin/users | List all users | Admin |
| GET | /api/admin/users/:id | Get user detail | Admin |
| PUT | /api/admin/users/:id | Update user | Admin |
| DELETE | /api/admin/users/:id | Delete user | Admin |

### Telegram Binding

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | /api/telegram/bind-code | Generate bind code | JWT |
| GET | /api/telegram/status | Check binding status | JWT |
| DELETE | /api/telegram/unbind | Unbind Telegram | JWT |

### Subscriptions

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | /api/subscriptions | List my subscriptions | JWT |
| POST | /api/subscriptions | Add subscription | JWT |
| PUT | /api/subscriptions/:id | Update subscription | JWT |
| DELETE | /api/subscriptions/:id | Delete subscription | JWT |

---

## Telegram Binding Flow

```
1. User logs in to web
2. User clicks "Bind Telegram"
3. System generates bind_code (e.g., "abc123"), valid for 10 minutes
4. Web displays: "Send /bind abc123 to @PttAlertorBot"
5. User sends message to Bot
6. Bot receives:
   - chat_id: 123456789
   - text: "/bind abc123"
7. System:
   - Find user with bind_code = "abc123"
   - Check if bind_code is not expired
   - Save telegram_chat_id = 123456789
   - Clear bind_code
8. Bot replies: "Bindingsuccess!"
9. Web shows: "Telegram bound successfully"
```

---

## Implementation Tasks

### Phase 1: Database & Auth

- [ ] Create migrations for users and subscriptions tables
- [ ] Implement user registration API
- [ ] Implement user login API (JWT)
- [ ] Implement auth middleware
- [ ] Implement role-based access control

### Phase 2: Telegram Binding

- [ ] Add /bind command to Telegram bot
- [ ] Implement bind code generation API
- [ ] Implement binding verification logic
- [ ] Implement unbind API

### Phase 3: Subscription Management

- [ ] Implement subscription CRUD APIs
- [ ] Migrate existing Redis user data to PostgreSQL
- [ ] Update Checker to read from PostgreSQL

### Phase 4: Admin Features

- [ ] Implement admin user list API
- [ ] Implement admin user management APIs
- [ ] Add admin dashboard page

### Phase 5: Frontend (Optional)

- [ ] Login/Register pages
- [ ] Subscription management page
- [ ] Telegram binding page
- [ ] Admin dashboard

---

## Data Architecture

### PostgreSQL + Redis Hybrid

```
┌─────────────────────────────────────────────────────────────┐
│                      Write Path                              │
│                                                             │
│   Web API ── Create/Update ──▶ PostgreSQL (Source of Truth) │
│                                      │                      │
│                                      ▼                      │
│                               Sync to Redis (Cache)         │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                      Read Path                               │
│                                                             │
│   Checker ── Query Subscriptions ──▶ Redis (Fast Cache)     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### Data Storage Responsibilities

| Layer | Purpose |
|-------|---------|
| PostgreSQL | Source of truth, web management, user accounts, persistence |
| Redis | Cache for high-frequency reads, Checker performance |

### Redis Data Structure (Keep existing format)

```
keyword:{board}:subs     → SET ["user1", "user2", ...]
author:{board}:subs      → SET ["user1", "user2", ...]
pushsum:{board}:subs     → SET ["user1", "user2", ...]
user:{account}           → JSON { profile, subscribes, ... }
```

### Sync Strategy: Write-through

```go
// When creating subscription via Web API
func (s *SubscriptionService) Create(sub Subscription) error {
    // 1. Write to PostgreSQL (source of truth)
    if err := s.postgres.Create(sub); err != nil {
        return err
    }

    // 2. Sync to Redis (cache)
    s.redis.AddSubscriber(sub.Board, sub.UserAccount, sub.SubType, sub.Value)

    return nil
}

// When deleting subscription
func (s *SubscriptionService) Delete(id int) error {
    // 1. Get subscription info first
    sub, _ := s.postgres.Find(id)

    // 2. Delete from PostgreSQL
    if err := s.postgres.Delete(id); err != nil {
        return err
    }

    // 3. Remove from Redis
    s.redis.RemoveSubscriber(sub.Board, sub.UserAccount, sub.SubType, sub.Value)

    return nil
}
```

### Checker (No changes needed)

```go
// Still reads from Redis - fast!
func Subscribers(board string) []string {
    return redis.SMEMBERS("keyword:" + board + ":subs")
}
```

---

## Migration Strategy

### From existing Redis users to new system

1. Keep existing Redis data and Telegram bot commands working
2. New web users: PostgreSQL + sync to Redis
3. Optional: Admin tool to import existing Redis users to PostgreSQL
4. Both systems coexist - Checker reads from same Redis cache

---

## Environment Variables (New)

```bash
# JWT
JWT_SECRET=your-jwt-secret-key
JWT_EXPIRES_IN=24h

# Admin (first admin account)
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=admin-password
```

---

## Notes

- Passwords must be hashed with bcrypt
- JWT tokens expire in 24 hours
- Bind codes expire in 10 minutes
- One user can only bind one Telegram account
- Admin can manage all users but cannot see passwords

# PostgreSQL Migration Plan

> DynamoDB to PostgreSQL (Normalized Design)

This document outlines the complete implementation plan for migrating from AWS DynamoDB to a self-hosted PostgreSQL database with normalized schema design.

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Database Schema Design](#database-schema-design)
3. [File Changes](#file-changes)
4. [Docker Compose Setup](#docker-compose-setup)
5. [Implementation Details](#implementation-details)
6. [Environment Variables](#environment-variables)
7. [Implementation Order](#implementation-order)

---

## Architecture Overview

### Current Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Current Data Flow                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Board                                                          │
│  ├── driver: DynamoDB  ──► Persistent Storage (Articles JSON)   │
│  └── cacher: Redis     ──► Cache (Board List Set)               │
│                                                                 │
│  Article                                                        │
│  └── driver: DynamoDB  ──► Persistent Storage (Article+Comments)│
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

Issues:
1. DynamoDB boards table stores Articles as JSON string (denormalized)
2. DynamoDB articles table stores Comments as JSON string (denormalized)
3. CheckBoardExist() function hardcodes new(DynamoDB)
```

### New Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         New Data Flow                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Board                                                          │
│  ├── driver: Postgres  ──► PostgreSQL (Normalized Tables)       │
│  └── cacher: Redis     ──► Cache (Board List Set) [Unchanged]   │
│                                                                 │
│  Article                                                        │
│  └── driver: Postgres  ──► PostgreSQL (Normalized Tables + JOIN)│
│                                                                 │
│  Docker Compose                                                 │
│  ├── postgres:15                                                │
│  └── redis:7                                                    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Database Schema Design

### Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                   Normalized Database Design                     │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐       ┌─────────────┐       ┌─────────────┐   │
│  │   boards    │       │  articles   │       │  comments   │   │
│  ├─────────────┤       ├─────────────┤       ├─────────────┤   │
│  │ name (PK)   │◄──────│ board_name  │       │ id (PK)     │   │
│  │ created_at  │  1:N  │ code (PK)   │◄──────│ article_code│   │
│  │ updated_at  │       │ id          │  1:N  │ tag         │   │
│  └─────────────┘       │ title       │       │ user_id     │   │
│                        │ link        │       │ content     │   │
│                        │ date        │       │ datetime    │   │
│                        │ author      │       │ created_at  │   │
│                        │ push_sum    │       └─────────────┘   │
│                        │ last_push_dt│                         │
│                        │ created_at  │                         │
│                        │ updated_at  │                         │
│                        └─────────────┘                         │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### SQL Schema

```sql
-- migrations/001_init.sql

-- Boards table
CREATE TABLE boards (
    name        VARCHAR(50) PRIMARY KEY,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- Articles table
CREATE TABLE articles (
    code               VARCHAR(30) PRIMARY KEY,
    id                 INTEGER NOT NULL,
    title              TEXT NOT NULL,
    link               TEXT,
    date               VARCHAR(20),
    author             VARCHAR(50),
    board_name         VARCHAR(50) NOT NULL REFERENCES boards(name) ON DELETE CASCADE,
    push_sum           INTEGER DEFAULT 0,
    last_push_datetime TIMESTAMP,
    created_at         TIMESTAMP DEFAULT NOW(),
    updated_at         TIMESTAMP DEFAULT NOW()
);

-- Comments table
CREATE TABLE comments (
    id            SERIAL PRIMARY KEY,
    article_code  VARCHAR(30) NOT NULL REFERENCES articles(code) ON DELETE CASCADE,
    tag           VARCHAR(10),
    user_id       VARCHAR(50),
    content       TEXT,
    datetime      TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_articles_board ON articles(board_name);
CREATE INDEX idx_articles_author ON articles(author);
CREATE INDEX idx_articles_id ON articles(id);
CREATE INDEX idx_comments_article ON comments(article_code);

-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to boards
CREATE TRIGGER boards_updated_at
    BEFORE UPDATE ON boards
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to articles
CREATE TRIGGER articles_updated_at
    BEFORE UPDATE ON articles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
```

---

## File Changes

### Files to Add

| File | Description |
|------|-------------|
| `docker-compose.yml` | Redis + PostgreSQL services |
| `migrations/001_init.sql` | Database schema |
| `connections/postgres.go` | PostgreSQL connection pool |
| `models/board/postgres.go` | board.Driver implementation |
| `models/article/postgres.go` | article.Driver implementation |

### Files to Modify

| File | Changes |
|------|---------|
| `models/models.go` | Switch to Postgres Driver |
| `models/board/board.go` | Fix CheckBoardExist() hardcoded DynamoDB |
| `models/board/redis.go` | Update to gomodule/redigo |
| `connections/redis.go` | Update to gomodule/redigo |
| `.env.example` | Add PostgreSQL settings |
| `go.mod` | Add pgx/v5, update redigo |

### Files to Remove (Optional)

| File | Reason |
|------|--------|
| `models/board/dynamodb.go` | No longer needed |
| `models/article/dynamodb.go` | No longer needed |

### Directory Structure

```
ptt-alertor/
│
├── docker-compose.yml              (NEW)
├── migrations/
│   └── 001_init.sql                (NEW)
│
├── connections/
│   ├── redis.go                    (MODIFY)
│   └── postgres.go                 (NEW)
│
├── models/
│   ├── board/
│   │   ├── board.go                (MODIFY)
│   │   ├── dynamodb.go             (DELETE or KEEP)
│   │   ├── redis.go                (MODIFY)
│   │   └── postgres.go             (NEW)
│   │
│   ├── article/
│   │   ├── dynamodb.go             (DELETE or KEEP)
│   │   └── postgres.go             (NEW)
│   │
│   └── models.go                   (MODIFY)
│
├── .env.example                    (MODIFY)
├── go.mod                          (MODIFY)
└── Dockerfile                      (MAYBE MODIFY)
```

---

## Docker Compose Setup

### docker-compose.yml

```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    container_name: ptt-alertor-postgres
    environment:
      POSTGRES_USER: ptt_alertor
      POSTGRES_PASSWORD: ptt_alertor_secret
      POSTGRES_DB: ptt_alertor
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ptt_alertor"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: ptt-alertor-redis
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  # Optional: Application service
  # app:
  #   build: .
  #   container_name: ptt-alertor-app
  #   depends_on:
  #     postgres:
  #       condition: service_healthy
  #     redis:
  #       condition: service_healthy
  #   env_file:
  #     - .env
  #   ports:
  #     - "9090:9090"

volumes:
  postgres_data:
  redis_data:
```

### Usage

```bash
# Start services
docker-compose up -d

# Check logs
docker-compose logs -f postgres
docker-compose logs -f redis

# Stop services
docker-compose down

# Stop and remove volumes (clean data)
docker-compose down -v
```

---

## Implementation Details

### Interface Definitions

#### board.Driver Interface

```go
type Driver interface {
    GetArticles(boardName string) article.Articles
    Save(boardName string, articles article.Articles) error
    Delete(boardName string) error
}
```

#### article.Driver Interface

```go
type Driver interface {
    Find(code string, article *Article)
    Save(a Article) error
    Delete(code string) error
}
```

### Implementation Comparison

#### board.Driver Methods

| Method | DynamoDB (Old) | PostgreSQL (New) |
|--------|----------------|------------------|
| `GetArticles(boardName)` | GetItem → JSON Unmarshal | `SELECT * FROM articles WHERE board_name = $1` |
| `Save(boardName, articles)` | PutItem (entire JSON) | `INSERT ... ON CONFLICT UPDATE` (per row) |
| `Delete(boardName)` | DeleteItem | `DELETE FROM articles WHERE board_name = $1` |

#### article.Driver Methods

| Method | DynamoDB (Old) | PostgreSQL (New) |
|--------|----------------|------------------|
| `Find(code, *Article)` | GetItem → Manual Unmarshal | `SELECT + JOIN comments` |
| `Save(Article)` | PutItem | `INSERT article + INSERT comments` (transaction) |
| `Delete(code)` | DeleteItem | `DELETE FROM articles` (CASCADE deletes comments) |

### Code Examples

#### connections/postgres.go

```go
package connections

import (
    "context"
    "fmt"
    "os"
    "sync"

    "github.com/jackc/pgx/v5/pgxpool"
)

var (
    pgPool     *pgxpool.Pool
    pgPoolOnce sync.Once
)

func Postgres() *pgxpool.Pool {
    pgPoolOnce.Do(func() {
        connString := fmt.Sprintf(
            "postgres://%s:%s@%s:%s/%s?pool_max_conns=%s",
            os.Getenv("PG_USER"),
            os.Getenv("PG_PASSWORD"),
            os.Getenv("PG_HOST"),
            os.Getenv("PG_PORT"),
            os.Getenv("PG_DATABASE"),
            os.Getenv("PG_POOL_MAX"),
        )

        var err error
        pgPool, err = pgxpool.New(context.Background(), connString)
        if err != nil {
            panic(fmt.Sprintf("Unable to connect to database: %v", err))
        }
    })
    return pgPool
}
```

#### board.Driver.GetArticles (PostgreSQL)

```go
func (p *Postgres) GetArticles(boardName string) (articles article.Articles) {
    ctx := context.Background()
    pool := connections.Postgres()

    rows, err := pool.Query(ctx, `
        SELECT code, id, title, link, date, author, push_sum, last_push_datetime
        FROM articles
        WHERE board_name = $1
        ORDER BY id DESC
    `, boardName)
    if err != nil {
        log.WithError(err).Error("PostgreSQL GetArticles Failed")
        return
    }
    defer rows.Close()

    for rows.Next() {
        var a article.Article
        var lastPushDT sql.NullTime

        err := rows.Scan(
            &a.Code, &a.ID, &a.Title, &a.Link,
            &a.Date, &a.Author, &a.PushSum, &lastPushDT,
        )
        if err != nil {
            log.WithError(err).Error("PostgreSQL Scan Failed")
            continue
        }

        if lastPushDT.Valid {
            a.LastPushDateTime = lastPushDT.Time
        }
        a.Board = boardName
        articles = append(articles, a)
    }

    return articles
}
```

#### article.Driver.Find (PostgreSQL with JOIN)

```go
func (p *Postgres) Find(code string, a *article.Article) {
    ctx := context.Background()
    pool := connections.Postgres()

    // Query article
    var lastPushDT sql.NullTime
    err := pool.QueryRow(ctx, `
        SELECT code, id, title, link, date, author, board_name, push_sum, last_push_datetime
        FROM articles WHERE code = $1
    `, code).Scan(
        &a.Code, &a.ID, &a.Title, &a.Link, &a.Date,
        &a.Author, &a.Board, &a.PushSum, &lastPushDT,
    )
    if err != nil {
        log.WithError(err).Warn("Article Not Found")
        return
    }

    if lastPushDT.Valid {
        a.LastPushDateTime = lastPushDT.Time
    }

    // Query comments
    rows, err := pool.Query(ctx, `
        SELECT tag, user_id, content, datetime
        FROM comments
        WHERE article_code = $1
        ORDER BY datetime
    `, code)
    if err != nil {
        log.WithError(err).Error("PostgreSQL Query Comments Failed")
        return
    }
    defer rows.Close()

    for rows.Next() {
        var c article.Comment
        var dt sql.NullTime

        rows.Scan(&c.Tag, &c.UserID, &c.Content, &dt)
        if dt.Valid {
            c.DateTime = dt.Time
        }
        a.Comments = append(a.Comments, c)
    }
}
```

#### article.Driver.Save (PostgreSQL with Transaction)

```go
func (p *Postgres) Save(a article.Article) error {
    ctx := context.Background()
    pool := connections.Postgres()

    tx, err := pool.Begin(ctx)
    if err != nil {
        return err
    }
    defer tx.Rollback(ctx)

    // Ensure board exists
    _, err = tx.Exec(ctx, `
        INSERT INTO boards (name) VALUES ($1)
        ON CONFLICT (name) DO NOTHING
    `, a.Board)
    if err != nil {
        return err
    }

    // Upsert article
    _, err = tx.Exec(ctx, `
        INSERT INTO articles (code, id, title, link, date, author, board_name, push_sum, last_push_datetime)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        ON CONFLICT (code) DO UPDATE SET
            title = EXCLUDED.title,
            push_sum = EXCLUDED.push_sum,
            last_push_datetime = EXCLUDED.last_push_datetime,
            updated_at = NOW()
    `, a.Code, a.ID, a.Title, a.Link, a.Date, a.Author, a.Board, a.PushSum, a.LastPushDateTime)
    if err != nil {
        return err
    }

    // Delete old comments
    _, err = tx.Exec(ctx, `DELETE FROM comments WHERE article_code = $1`, a.Code)
    if err != nil {
        return err
    }

    // Insert new comments
    for _, c := range a.Comments {
        _, err = tx.Exec(ctx, `
            INSERT INTO comments (article_code, tag, user_id, content, datetime)
            VALUES ($1, $2, $3, $4, $5)
        `, a.Code, c.Tag, c.UserID, c.Content, c.DateTime)
        if err != nil {
            return err
        }
    }

    return tx.Commit(ctx)
}
```

#### models/models.go Changes

```go
package models

import (
    "github.com/Ptt-Alertor/ptt-alertor/models/article"
    "github.com/Ptt-Alertor/ptt-alertor/models/board"
    "github.com/Ptt-Alertor/ptt-alertor/models/user"
)

var User = func() *user.User {
    return user.NewUser(new(user.Redis))
}

// Changed from DynamoDB to Postgres
var Article = func() *article.Article {
    return article.NewArticle(new(article.Postgres))
}

// Changed from DynamoDB to Postgres (Redis cacher unchanged)
var Board = func() *board.Board {
    return board.NewBoard(new(board.Postgres), new(board.Redis))
}
```

---

## Environment Variables

### .env.example (Updated)

```env
# Application
APP_HOST=
APP_WS_HOST=
AUTH_USER=
AUTH_PW=
BOARD_HIGH=

# PostgreSQL (NEW)
PG_HOST=localhost
PG_PORT=5432
PG_USER=ptt_alertor
PG_PASSWORD=ptt_alertor_secret
PG_DATABASE=ptt_alertor
PG_POOL_MAX=10

# Redis (Updated variable names)
REDIS_HOST=localhost
REDIS_PORT=6379

# LINE
LINE_CHANNEL_SECRET=
LINE_CHANNEL_ACCESSTOKEN=
LINE_CLIENT_ID=
LINE_CLIENT_SECRET=

# Telegram
TELEGRAM_TOKEN=

# Mailgun
MAILGUN_DOMAIN=
MAILGUN_APIKEY=
MAILGUN_PUBLIC_APIKEY=

# Messenger
MESSENGER_ACCESSTOKEN=
MESSENGER_VERIFYTOKEN=

# AWS (Can be removed after migration)
# AWS_REGION=
# S3_DOMAIN=
```

---

## Implementation Order

### Step-by-Step Guide

```
Step 1: Docker Compose + Database Schema
├── Create docker-compose.yml
├── Create migrations/001_init.sql
├── Run: docker-compose up -d
└── Verify: psql connection test

Step 2: PostgreSQL Connection Pool
├── Create connections/postgres.go
├── Add pgx/v5 to go.mod
└── Test: verify connection works

Step 3: Article PostgreSQL Driver
├── Create models/article/postgres.go
├── Implement: Find(), Save(), Delete()
└── Test: unit tests for article operations

Step 4: Board PostgreSQL Driver
├── Create models/board/postgres.go
├── Implement: GetArticles(), Save(), Delete()
└── Test: unit tests for board operations

Step 5: Update models/models.go
├── Change Article driver to Postgres
├── Change Board driver to Postgres
└── Keep Redis cacher unchanged

Step 6: Fix Hardcoded References
├── Update models/board/board.go
│   └── Fix CheckBoardExist() function
└── Remove hardcoded new(DynamoDB)

Step 7: Update Redis Import Path
├── Update connections/redis.go
│   └── github.com/garyburd/redigo → github.com/gomodule/redigo
├── Update models/board/redis.go
└── Update all other files using redigo

Step 8: Update Configuration
├── Update .env.example
├── Update go.mod (go mod tidy)
└── Test: full integration test

Step 9: Cleanup (Optional)
├── Remove models/board/dynamodb.go
├── Remove models/article/dynamodb.go
└── Remove AWS SDK dependencies from go.mod
```

### Verification Checklist

- [ ] `docker-compose up -d` starts successfully
- [ ] PostgreSQL accepts connections
- [ ] Redis accepts connections
- [ ] Database schema created correctly
- [ ] `go build` succeeds
- [ ] Application starts without errors
- [ ] Board list works (Redis cacher)
- [ ] Article save/find works (PostgreSQL)
- [ ] Comments are stored correctly
- [ ] All notification channels work

---

## Dependencies

### go.mod Changes

```go
// Add
github.com/jackc/pgx/v5 v5.x.x

// Update
github.com/garyburd/redigo → github.com/gomodule/redigo v1.9.x

// Remove (optional, after cleanup)
github.com/aws/aws-sdk-go
```

### Install Commands

```bash
# Add pgx
go get github.com/jackc/pgx/v5

# Update redigo
go get github.com/gomodule/redigo

# Clean up
go mod tidy
```

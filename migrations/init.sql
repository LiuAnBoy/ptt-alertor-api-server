-- PTT Alertor Database Schema
-- Consolidated initialization script

-- ============================================
-- 1. Boards table
-- ============================================
CREATE TABLE IF NOT EXISTS boards (
    name        VARCHAR(50) PRIMARY KEY,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- ============================================
-- 2. Articles table
-- ============================================
CREATE TABLE IF NOT EXISTS articles (
    code               VARCHAR(50) PRIMARY KEY,
    id                 INTEGER NOT NULL,
    title              TEXT NOT NULL,
    link               TEXT,
    date               VARCHAR(50),
    author             VARCHAR(50),
    board_name         VARCHAR(50) NOT NULL REFERENCES boards(name) ON DELETE CASCADE,
    push_sum           INTEGER DEFAULT 0,
    last_push_datetime TIMESTAMP,
    positive_count     INTEGER DEFAULT 0,
    negative_count     INTEGER DEFAULT 0,
    neutral_count      INTEGER DEFAULT 0,
    created_at         TIMESTAMP DEFAULT NOW(),
    updated_at         TIMESTAMP DEFAULT NOW()
);

-- ============================================
-- 3. Comments table
-- ============================================
CREATE TABLE IF NOT EXISTS comments (
    id            SERIAL PRIMARY KEY,
    article_code  VARCHAR(50) NOT NULL REFERENCES articles(code) ON DELETE CASCADE,
    tag           VARCHAR(10),
    user_id       VARCHAR(50),
    content       TEXT,
    datetime      TIMESTAMP,
    created_at    TIMESTAMP DEFAULT NOW()
);

-- ============================================
-- 4. Role Limits table (must be created before users)
-- ============================================
CREATE TABLE IF NOT EXISTS role_limits (
    role                VARCHAR(20) PRIMARY KEY,
    max_subscriptions   INTEGER NOT NULL DEFAULT 3,
    description         VARCHAR(100),
    created_at          TIMESTAMP DEFAULT NOW(),
    updated_at          TIMESTAMP DEFAULT NOW()
);

-- Insert default roles
INSERT INTO role_limits (role, max_subscriptions, description) VALUES
('admin', -1, '管理員，無限制'),
('vip', 20, 'VIP 用戶'),
('user', 3, '一般用戶')
ON CONFLICT (role) DO NOTHING;

-- ============================================
-- 5. Users table
-- ============================================
CREATE TABLE IF NOT EXISTS users (
    id          SERIAL PRIMARY KEY,
    email       VARCHAR(255) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,
    role        VARCHAR(20) DEFAULT 'user' REFERENCES role_limits(role),
    enabled     BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW()
);

-- ============================================
-- 6. Subscriptions table
-- ============================================
CREATE TABLE IF NOT EXISTS subscriptions (
    id          SERIAL PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    board       VARCHAR(50) NOT NULL,
    sub_type    VARCHAR(20) NOT NULL CHECK (sub_type IN ('keyword', 'author', 'pushsum')),
    value       VARCHAR(255) NOT NULL,
    enabled     BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMP DEFAULT NOW(),
    updated_at  TIMESTAMP DEFAULT NOW(),
    UNIQUE(user_id, board, sub_type, value)
);

-- ============================================
-- 7. Notification Bindings table
-- ============================================
CREATE TABLE IF NOT EXISTS notification_bindings (
    id                   SERIAL PRIMARY KEY,
    user_id              INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    service              VARCHAR(32) NOT NULL,
    service_id           VARCHAR(128) NOT NULL,
    bind_code            VARCHAR(64),
    bind_code_expires_at TIMESTAMP,
    enabled              BOOLEAN DEFAULT TRUE,
    created_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at           TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, service),
    UNIQUE(service, service_id)
);

-- ============================================
-- 8. Subscription Stats table (for analytics)
-- ============================================
CREATE TABLE IF NOT EXISTS subscription_stats (
    id          SERIAL PRIMARY KEY,
    board       VARCHAR(50) NOT NULL,
    sub_type    VARCHAR(20) NOT NULL CHECK (sub_type IN ('keyword', 'author', 'pushsum')),
    value       VARCHAR(255) NOT NULL,
    count       INTEGER DEFAULT 0,
    updated_at  TIMESTAMP DEFAULT NOW(),
    UNIQUE(board, sub_type, value)
);

-- ============================================
-- 9. Indexes
-- ============================================
-- Articles indexes
CREATE INDEX IF NOT EXISTS idx_articles_board ON articles(board_name);
CREATE INDEX IF NOT EXISTS idx_articles_author ON articles(author);
CREATE INDEX IF NOT EXISTS idx_articles_id ON articles(id);

-- Comments indexes
CREATE INDEX IF NOT EXISTS idx_comments_article ON comments(article_code);

-- Users indexes
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Subscriptions indexes
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_board ON subscriptions(board);
CREATE INDEX IF NOT EXISTS idx_subscriptions_board_type ON subscriptions(board, sub_type);

-- Notification bindings indexes
CREATE INDEX IF NOT EXISTS idx_notification_bindings_user_id ON notification_bindings(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_bindings_service ON notification_bindings(service);
CREATE INDEX IF NOT EXISTS idx_notification_bindings_service_id ON notification_bindings(service, service_id);

-- Subscription stats indexes
CREATE INDEX IF NOT EXISTS idx_subscription_stats_type_count ON subscription_stats(sub_type, count DESC);
CREATE INDEX IF NOT EXISTS idx_subscription_stats_board ON subscription_stats(board);

-- ============================================
-- 10. Triggers
-- ============================================
-- Updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply trigger to boards
DROP TRIGGER IF EXISTS boards_updated_at ON boards;
CREATE TRIGGER boards_updated_at
    BEFORE UPDATE ON boards
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to role_limits
DROP TRIGGER IF EXISTS role_limits_updated_at ON role_limits;
CREATE TRIGGER role_limits_updated_at
    BEFORE UPDATE ON role_limits
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to articles
DROP TRIGGER IF EXISTS articles_updated_at ON articles;
CREATE TRIGGER articles_updated_at
    BEFORE UPDATE ON articles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to users
DROP TRIGGER IF EXISTS users_updated_at ON users;
CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to subscriptions
DROP TRIGGER IF EXISTS subscriptions_updated_at ON subscriptions;
CREATE TRIGGER subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to notification_bindings
DROP TRIGGER IF EXISTS notification_bindings_updated_at ON notification_bindings;
CREATE TRIGGER notification_bindings_updated_at
    BEFORE UPDATE ON notification_bindings
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply trigger to subscription_stats
DROP TRIGGER IF EXISTS subscription_stats_updated_at ON subscription_stats;
CREATE TRIGGER subscription_stats_updated_at
    BEFORE UPDATE ON subscription_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

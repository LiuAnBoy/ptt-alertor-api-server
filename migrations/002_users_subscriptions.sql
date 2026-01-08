-- Users and Subscriptions Schema
-- For web-based user management

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id                    SERIAL PRIMARY KEY,
    email                 VARCHAR(255) UNIQUE NOT NULL,
    password_hash         VARCHAR(255) NOT NULL,
    role                  VARCHAR(20) DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    telegram_chat_id      BIGINT,
    telegram_bind_code    VARCHAR(32),
    bind_code_expires_at  TIMESTAMP,
    enabled               BOOLEAN DEFAULT TRUE,
    created_at            TIMESTAMP DEFAULT NOW(),
    updated_at            TIMESTAMP DEFAULT NOW()
);

-- Subscriptions table
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

-- Indexes
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_telegram_chat_id ON users(telegram_chat_id);
CREATE INDEX IF NOT EXISTS idx_users_bind_code ON users(telegram_bind_code);
CREATE INDEX IF NOT EXISTS idx_subscriptions_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subscriptions_board ON subscriptions(board);
CREATE INDEX IF NOT EXISTS idx_subscriptions_board_type ON subscriptions(board, sub_type);

-- Apply updated_at trigger to users
DROP TRIGGER IF EXISTS users_updated_at ON users;
CREATE TRIGGER users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- Apply updated_at trigger to subscriptions
DROP TRIGGER IF EXISTS subscriptions_updated_at ON subscriptions;
CREATE TRIGGER subscriptions_updated_at
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

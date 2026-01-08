-- Create notification_bindings table
CREATE TABLE IF NOT EXISTS notification_bindings (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    service VARCHAR(32) NOT NULL,
    service_id VARCHAR(128) NOT NULL,
    bind_code VARCHAR(64),
    bind_code_expires_at TIMESTAMP,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, service),
    UNIQUE(service, service_id)
);

-- Migrate existing telegram data from users to notification_bindings
INSERT INTO notification_bindings (user_id, service, service_id, bind_code, bind_code_expires_at, created_at, updated_at)
SELECT id, 'telegram', telegram_chat_id::VARCHAR, telegram_bind_code, bind_code_expires_at, created_at, updated_at
FROM users
WHERE telegram_chat_id IS NOT NULL
ON CONFLICT DO NOTHING;

-- Remove telegram columns from users table
ALTER TABLE users DROP COLUMN IF EXISTS telegram_chat_id;
ALTER TABLE users DROP COLUMN IF EXISTS telegram_bind_code;
ALTER TABLE users DROP COLUMN IF EXISTS bind_code_expires_at;

-- Rename password_hash to password
ALTER TABLE users RENAME COLUMN password_hash TO password;

-- Create indexes for faster lookups
CREATE INDEX IF NOT EXISTS idx_notification_bindings_user_id ON notification_bindings(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_bindings_service ON notification_bindings(service);
CREATE INDEX IF NOT EXISTS idx_notification_bindings_service_id ON notification_bindings(service, service_id);

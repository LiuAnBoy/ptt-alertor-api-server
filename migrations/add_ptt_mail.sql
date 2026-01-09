-- Add PTT mail feature
-- 1. PTT account binding table
-- 2. Add mail template columns to subscriptions

-- 1. PTT 帳號綁定
CREATE TABLE IF NOT EXISTS ptt_accounts (
    id                      SERIAL PRIMARY KEY,
    user_id                 INTEGER UNIQUE NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ptt_username            VARCHAR(50) NOT NULL,
    ptt_password_encrypted  TEXT NOT NULL,
    created_at              TIMESTAMP DEFAULT NOW(),
    updated_at              TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ptt_accounts_user_id ON ptt_accounts(user_id);

-- 2. Subscriptions 加信件範本欄位
ALTER TABLE subscriptions 
ADD COLUMN IF NOT EXISTS mail_subject VARCHAR(100),
ADD COLUMN IF NOT EXISTS mail_content TEXT;

-- 3. Trigger for updated_at
DROP TRIGGER IF EXISTS ptt_accounts_updated_at ON ptt_accounts;
CREATE TRIGGER ptt_accounts_updated_at
    BEFORE UPDATE ON ptt_accounts
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

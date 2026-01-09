-- Add role_limits table for managing subscription limits per role
-- This replaces the hardcoded MaxSubscriptionsForUser constant

-- 1. Create role_limits table
CREATE TABLE IF NOT EXISTS role_limits (
    role                VARCHAR(20) PRIMARY KEY,
    max_subscriptions   INTEGER NOT NULL DEFAULT 3,
    description         VARCHAR(100),
    created_at          TIMESTAMP DEFAULT NOW(),
    updated_at          TIMESTAMP DEFAULT NOW()
);

-- 2. Insert default roles
INSERT INTO role_limits (role, max_subscriptions, description) VALUES
('admin', -1, '管理員，無限制'),
('vip', 20, 'VIP 用戶'),
('user', 3, '一般用戶')
ON CONFLICT (role) DO NOTHING;

-- 3. Add trigger for updated_at
DROP TRIGGER IF EXISTS role_limits_updated_at ON role_limits;
CREATE TRIGGER role_limits_updated_at
    BEFORE UPDATE ON role_limits
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- 4. Change users.role from CHECK constraint to Foreign Key
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_fk FOREIGN KEY (role) REFERENCES role_limits(role);

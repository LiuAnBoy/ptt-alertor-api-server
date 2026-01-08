-- Add subscription_stats table for analytics
-- Run this migration manually: psql -d your_database -f add_subscription_stats.sql

-- ============================================
-- 1. Create subscription_stats table
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
-- 2. Create indexes
-- ============================================
CREATE INDEX IF NOT EXISTS idx_subscription_stats_type_count ON subscription_stats(sub_type, count DESC);
CREATE INDEX IF NOT EXISTS idx_subscription_stats_board ON subscription_stats(board);

-- ============================================
-- 3. Create updated_at trigger
-- ============================================
DROP TRIGGER IF EXISTS subscription_stats_updated_at ON subscription_stats;
CREATE TRIGGER subscription_stats_updated_at
    BEFORE UPDATE ON subscription_stats
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ============================================
-- 4. Initialize stats from existing subscriptions
-- ============================================
-- Note: This basic initialization only handles simple values
-- Complex values (regexp, &, !) need to be processed by the application

-- Insert keyword stats (simple keywords only, excluding regexp/&/!)
INSERT INTO subscription_stats (board, sub_type, value, count)
SELECT
    board,
    sub_type,
    value,
    COUNT(*) as count
FROM subscriptions
WHERE sub_type IN ('author', 'pushsum')
   OR (sub_type = 'keyword'
       AND value NOT LIKE 'regexp:%'
       AND value NOT LIKE '!%'
       AND value NOT LIKE '%&%')
GROUP BY board, sub_type, value
ON CONFLICT (board, sub_type, value)
DO UPDATE SET count = EXCLUDED.count;

-- For keywords with & (AND logic), we need application-level processing
-- This will be handled by the initialization script

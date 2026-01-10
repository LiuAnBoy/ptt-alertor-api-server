-- Add 'article' to subscriptions sub_type CHECK constraint
-- This allows users to subscribe to specific article comment tracking

-- Drop existing constraint and add new one with 'article' type
ALTER TABLE subscriptions
DROP CONSTRAINT IF EXISTS subscriptions_sub_type_check;

ALTER TABLE subscriptions
ADD CONSTRAINT subscriptions_sub_type_check
CHECK (sub_type IN ('keyword', 'author', 'pushsum', 'article'));

-- Note: subscription_stats does NOT need 'article' type
-- because article tracking stats are not meaningful (articles are ephemeral and personal)

-- Add comment statistics columns to articles table
-- These columns track positive (推), negative (噓), and neutral (→) comment counts

ALTER TABLE articles
ADD COLUMN IF NOT EXISTS positive_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS negative_count INTEGER DEFAULT 0,
ADD COLUMN IF NOT EXISTS neutral_count INTEGER DEFAULT 0;

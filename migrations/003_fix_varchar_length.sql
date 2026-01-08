-- Fix varchar length issue
ALTER TABLE articles ALTER COLUMN date TYPE VARCHAR(50);
ALTER TABLE articles ALTER COLUMN code TYPE VARCHAR(50);

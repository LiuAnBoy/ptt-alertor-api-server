-- Add id column to role_limits table
-- Existing rows will get id 1, 2, 3 based on creation order

-- First add the column as nullable
ALTER TABLE role_limits ADD COLUMN IF NOT EXISTS id SERIAL;

-- Update existing rows with sequential IDs based on created_at order
WITH numbered AS (
    SELECT role, ROW_NUMBER() OVER (ORDER BY created_at) as rn
    FROM role_limits
)
UPDATE role_limits
SET id = numbered.rn
FROM numbered
WHERE role_limits.role = numbered.role;

-- Make id the primary key (if not already)
-- Note: If role is already primary key, you may need to drop it first
-- ALTER TABLE role_limits DROP CONSTRAINT IF EXISTS role_limits_pkey;
-- ALTER TABLE role_limits ADD PRIMARY KEY (id);

-- Add unique constraint on role
-- ALTER TABLE role_limits ADD CONSTRAINT role_limits_role_unique UNIQUE (role);

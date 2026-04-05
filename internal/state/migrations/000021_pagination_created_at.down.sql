DROP INDEX IF EXISTS idx_groups_created_at_id;
DROP INDEX IF EXISTS idx_policies_created_at_id;
ALTER TABLE groups DROP COLUMN IF EXISTS created_at;
ALTER TABLE policies DROP COLUMN IF EXISTS created_at;

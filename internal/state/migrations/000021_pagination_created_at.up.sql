-- S021-2-3: Add created_at columns to groups and policies for cursor-based pagination.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE policies ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_groups_created_at_id ON groups(created_at, id);
CREATE INDEX IF NOT EXISTS idx_policies_created_at_id ON policies(created_at, id);

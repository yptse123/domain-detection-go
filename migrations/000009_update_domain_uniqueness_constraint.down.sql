-- Drop the new constraint
DROP INDEX IF EXISTS idx_domains_user_name_region_unique;

-- Recreate the old one if needed
CREATE UNIQUE INDEX IF NOT EXISTS idx_domain_user_name ON domains(user_id, name);
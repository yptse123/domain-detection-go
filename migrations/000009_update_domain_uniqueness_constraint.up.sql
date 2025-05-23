-- First drop any existing uniqueness constraint (if there is one)
DROP INDEX IF EXISTS idx_domain_user_name;

-- Create a new unique index on user_id, name, and region
CREATE UNIQUE INDEX idx_domains_user_name_region_unique ON domains (user_id, LOWER(name), region);
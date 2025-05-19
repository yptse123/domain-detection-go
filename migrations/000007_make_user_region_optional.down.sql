-- migrations/000007_make_user_region_optional.down.sql
ALTER TABLE users ALTER COLUMN region SET NOT NULL;
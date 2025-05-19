-- migrations/000007_make_user_region_optional.up.sql
ALTER TABLE users ALTER COLUMN region DROP NOT NULL;
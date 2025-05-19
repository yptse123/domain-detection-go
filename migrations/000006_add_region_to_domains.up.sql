-- filename: migrations/000006_add_region_to_domains.up.sql
ALTER TABLE domains ADD COLUMN region VARCHAR(10);
-- filename: migrations/000006_add_region_to_domains.down.sql
ALTER TABLE domains DROP COLUMN region;
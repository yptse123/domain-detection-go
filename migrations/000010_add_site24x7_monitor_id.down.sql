-- filename: migrations/000010_add_site24x7_monitor_id.down.sql
DROP INDEX IF EXISTS idx_domains_site24x7_monitor_id;
ALTER TABLE domains DROP COLUMN site24x7_monitor_id;
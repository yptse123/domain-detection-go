ALTER TABLE domains ADD COLUMN site24x7_monitor_id VARCHAR(255);

CREATE INDEX idx_domains_site24x7_monitor_id ON domains(site24x7_monitor_id);
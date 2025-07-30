ALTER TABLE domains ADD COLUMN is_deep_check BOOLEAN DEFAULT false;

CREATE INDEX idx_domains_is_deep_check ON domains(is_deep_check);
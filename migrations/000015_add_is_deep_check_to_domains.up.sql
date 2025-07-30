ALTER TABLE domains ADD COLUMN is_deep_check BOOLEAN DEFAULT true;

CREATE INDEX idx_domains_is_deep_check ON domains(is_deep_check);
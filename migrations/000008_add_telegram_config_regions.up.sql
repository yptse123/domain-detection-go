CREATE TABLE telegram_config_regions (
    telegram_config_id INTEGER NOT NULL REFERENCES telegram_configs(id) ON DELETE CASCADE,
    region_code VARCHAR(10) NOT NULL REFERENCES regions(code) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY(telegram_config_id, region_code)
);

CREATE INDEX idx_telegram_config_regions_config_id ON telegram_config_regions(telegram_config_id);
CREATE INDEX idx_telegram_config_regions_region_code ON telegram_config_regions(region_code);
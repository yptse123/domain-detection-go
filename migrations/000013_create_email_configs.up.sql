CREATE TABLE email_configs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email_address VARCHAR(255) NOT NULL,
    email_name VARCHAR(255),
    language VARCHAR(10) DEFAULT 'en',
    is_active BOOLEAN DEFAULT true,
    notify_on_down BOOLEAN DEFAULT true,
    notify_on_up BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id, email_address)
);

CREATE TABLE email_config_regions (
    email_config_id INTEGER NOT NULL REFERENCES email_configs(id) ON DELETE CASCADE,
    region_code VARCHAR(10) NOT NULL REFERENCES regions(code) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY(email_config_id, region_code)
);

CREATE INDEX idx_email_configs_user_id ON email_configs(user_id);
CREATE INDEX idx_email_configs_active ON email_configs(is_active);
CREATE INDEX idx_email_config_regions_config_id ON email_config_regions(email_config_id);
CREATE INDEX idx_email_config_regions_region_code ON email_config_regions(region_code);
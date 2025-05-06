-- Store telegram groups/channels configuration
CREATE TABLE telegram_configs (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id VARCHAR(255) NOT NULL,
    chat_name VARCHAR(255),
    is_active BOOLEAN DEFAULT true,
    notify_on_down BOOLEAN DEFAULT true,
    notify_on_up BOOLEAN DEFAULT true,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Track notification history to avoid duplicates
CREATE TABLE notification_history (
    id SERIAL PRIMARY KEY,
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    telegram_config_id INTEGER NOT NULL REFERENCES telegram_configs(id) ON DELETE CASCADE,
    status_code INTEGER NOT NULL,
    error_code INTEGER,
    error_description TEXT,
    notified_at TIMESTAMP NOT NULL DEFAULT NOW(),
    notification_type VARCHAR(50) NOT NULL -- 'down', 'up', 'performance', etc.
);

-- Add index for efficient lookups
CREATE INDEX idx_notification_history_domain_type ON notification_history(domain_id, notification_type);
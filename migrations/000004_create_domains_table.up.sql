CREATE TABLE IF NOT EXISTS domains (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    active BOOLEAN DEFAULT TRUE,
    interval INTEGER DEFAULT 20, -- Default to 20 minutes
    last_status INTEGER DEFAULT 0, -- HTTP status code
    monitor_guid VARCHAR(255) DEFAULT NULL,
    error_code INTEGER DEFAULT 0,
    total_time INTEGER DEFAULT 0,
    error_description TEXT DEFAULT '',
    last_check TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Add indexes for performance
CREATE INDEX IF NOT EXISTS idx_domains_user_id ON domains(user_id);
CREATE INDEX IF NOT EXISTS idx_domains_name ON domains(name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_domain_user_name ON domains(user_id, name);

-- Create user_settings table to store domain limits
CREATE TABLE IF NOT EXISTS user_settings (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    domain_limit INTEGER DEFAULT 100,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
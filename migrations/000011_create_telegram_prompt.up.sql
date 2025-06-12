-- Add language column to telegram_configs
ALTER TABLE telegram_configs ADD COLUMN language VARCHAR(10) DEFAULT 'en';
CREATE INDEX idx_telegram_configs_language ON telegram_configs(language);

-- Create telegram_prompts table
CREATE TABLE telegram_prompts (
    id SERIAL PRIMARY KEY,
    key VARCHAR(100) NOT NULL,
    language VARCHAR(10) NOT NULL DEFAULT 'en',
    message TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(key, language)
);

CREATE INDEX idx_telegram_prompts_key ON telegram_prompts(key);
CREATE INDEX idx_telegram_prompts_language ON telegram_prompts(language); 
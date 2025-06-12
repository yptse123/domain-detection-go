-- Drop the new indexes
DROP INDEX IF EXISTS idx_telegram_prompts_messages;
DROP INDEX IF EXISTS idx_telegram_prompts_key;

-- Drop the updated table
DROP TABLE IF EXISTS telegram_prompts;

-- Recreate the original table structure from migration 11
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
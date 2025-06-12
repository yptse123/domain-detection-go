DROP TABLE IF EXISTS telegram_prompts;

-- Create telegram_prompts table with JSON for all languages
CREATE TABLE telegram_prompts (
    id SERIAL PRIMARY KEY,
    prompt_key VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    messages JSONB NOT NULL DEFAULT '{}',  -- Store all language messages in JSON
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_telegram_prompts_key ON telegram_prompts(prompt_key);
CREATE INDEX idx_telegram_prompts_messages ON telegram_prompts USING GIN (messages); 
-- Drop indexes
DROP INDEX IF EXISTS idx_telegram_prompts_language;
DROP INDEX IF EXISTS idx_telegram_prompts_key;

-- Drop table
DROP TABLE IF EXISTS telegram_prompts;

-- Drop language column from telegram_configs
DROP INDEX IF EXISTS idx_telegram_configs_language;
ALTER TABLE telegram_configs DROP COLUMN IF EXISTS language; 
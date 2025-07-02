ALTER TABLE notification_history 
ADD COLUMN email_config_id INTEGER REFERENCES email_configs(id) ON DELETE CASCADE;

-- Make telegram_config_id nullable since we now have email_config_id too
ALTER TABLE notification_history ALTER COLUMN telegram_config_id DROP NOT NULL;

-- Add constraint to ensure at least one of telegram_config_id or email_config_id is set
ALTER TABLE notification_history 
ADD CONSTRAINT check_notification_config 
CHECK ((telegram_config_id IS NOT NULL AND email_config_id IS NULL) OR 
       (telegram_config_id IS NULL AND email_config_id IS NOT NULL));

CREATE INDEX idx_notification_history_email_config ON notification_history(email_config_id);
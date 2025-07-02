DROP INDEX IF EXISTS idx_notification_history_email_config;
ALTER TABLE notification_history DROP CONSTRAINT IF EXISTS check_notification_config;
ALTER TABLE notification_history DROP COLUMN email_config_id;
ALTER TABLE notification_history ALTER COLUMN telegram_config_id SET NOT NULL;
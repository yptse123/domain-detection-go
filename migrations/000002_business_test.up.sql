-- Insert a business user with username 'test'
-- Default password is '123qwe' (this is a bcrypt hash)
-- Change this password after first login in production!
INSERT INTO users (
    username, 
    password_hash, 
    email, 
    two_factor_enabled,
    region
) VALUES (
    'test',
    -- This is a bcrypt hash for '123qwe'
    '$2a$12$9.9NEk7Ao6WXNEP5EpCO0ukfpvpyueuOr5nWhshK/PQWTA9BbUDSK',
    'test@example.com',
    FALSE,
    'US'
) ON CONFLICT (username) DO NOTHING;
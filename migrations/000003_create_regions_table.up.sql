CREATE TABLE IF NOT EXISTS regions (
    code VARCHAR(10) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Insert initial regions
INSERT INTO regions (code, name, is_active) VALUES
('CN', '中国 (China)', TRUE),
('VN', 'Việt Nam (Vietnam)', TRUE),
('ID', 'Indonesia', TRUE),
('IN', 'India', TRUE),
('TH', 'ประเทศไทย (Thailand)', TRUE),
('JP', '日本 (Japan)', TRUE),
('KR', '한국 (Korea)', TRUE);
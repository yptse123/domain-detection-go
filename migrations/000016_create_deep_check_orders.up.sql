CREATE TABLE deep_check_orders (
    id SERIAL PRIMARY KEY,
    order_id VARCHAR(255) NOT NULL UNIQUE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    domain_id INTEGER NOT NULL REFERENCES domains(id) ON DELETE CASCADE,
    domain_name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE,
    callback_received BOOLEAN DEFAULT false,
    callback_data JSONB,
    INDEX(order_id),
    INDEX(user_id),
    INDEX(domain_id),
    INDEX(status)
);

CREATE INDEX idx_deep_check_orders_order_id ON deep_check_orders(order_id);
CREATE INDEX idx_deep_check_orders_user_domain ON deep_check_orders(user_id, domain_id);
CREATE INDEX idx_deep_check_orders_status ON deep_check_orders(status);
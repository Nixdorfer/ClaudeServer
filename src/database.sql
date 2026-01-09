CREATE TABLE messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id VARCHAR(36) NOT NULL,
    exchange_number INTEGER NOT NULL,
    request TEXT NOT NULL,
    response TEXT,
    receive_time TIMESTAMP NOT NULL,
    send_time TIMESTAMP,
    response_time TIMESTAMP,
    duration DOUBLE PRECISION,
    request_tokens INTEGER,
    response_tokens INTEGER,
    tokens INTEGER,
    status VARCHAR(20) NOT NULL DEFAULT 'processing',
    notice TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_conversation_id ON messages(conversation_id);
CREATE INDEX IF NOT EXISTS idx_receive_time ON messages(receive_time DESC);
CREATE INDEX IF NOT EXISTS idx_status ON messages(status);
CREATE INDEX IF NOT EXISTS idx_conversation_exchange ON messages(conversation_id, exchange_number);

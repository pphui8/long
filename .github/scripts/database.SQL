-- 1. Users Table
CREATE TABLE users (
    username VARCHAR(50) PRIMARY KEY,
    password_hash VARCHAR(255) NOT NULL
);

-- 2. Conversations Table
CREATE TABLE conversations (
    -- Serial is an auto-incrementing integer: 1, 2, 3...
    id SERIAL PRIMARY KEY, 
    username VARCHAR(50) REFERENCES users(username) 
        ON UPDATE CASCADE 
        ON DELETE CASCADE,
    title VARCHAR(255) DEFAULT 'New Chat',
    summary TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 3. Messages Table
CREATE TABLE messages (
    id SERIAL PRIMARY KEY,
    conversation_id INT REFERENCES conversations(id) 
        ON DELETE CASCADE,
    
    role VARCHAR(20) NOT NULL, -- 'system', 'user', 'assistant'
    content TEXT NOT NULL,
    token_count INT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- Indices for faster lookups as the chat grows
CREATE INDEX idx_conv_username ON conversations(username);
CREATE INDEX idx_msg_conv_id ON messages(conversation_id);
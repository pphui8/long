CREATE TABLE IF NOT EXISTS conversation_summaries (
    id SERIAL PRIMARY KEY,
    conversation_id INT NOT NULL REFERENCES conversations(id)
        ON DELETE CASCADE,
    summary TEXT NOT NULL,
    summarized_through_message_id INT NOT NULL REFERENCES messages(id)
        ON DELETE CASCADE,
    summarized_token_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (conversation_id, summarized_through_message_id)
);

CREATE INDEX IF NOT EXISTS idx_conv_summary_latest
    ON conversation_summaries(conversation_id, summarized_through_message_id DESC);

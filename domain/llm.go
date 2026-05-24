package domain

import "time"

type LLMRequest struct {
	ConversationID *int   `json:"conversation_id"` // Optional: if provided, continue the conversation
	Model          string `json:"model" binding:"required"`
	Prompt         string `json:"prompt" binding:"required"`
}

type Conversation struct {
	ID            int       `json:"id"`
	Username      string    `json:"username"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	CreatedAt     time.Time `json:"created_at"`
	LastMessageAt time.Time `json:"last_message_at"`
}

type ConversationSummary struct {
	ID                         int       `json:"id"`
	ConversationID             int       `json:"conversation_id"`
	Summary                    string    `json:"summary"`
	SummarizedThroughMessageID int       `json:"summarized_through_message_id"`
	SummarizedTokenCount       int       `json:"summarized_token_count"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

type Message struct {
	ID             int       `json:"id"`
	ConversationID int       `json:"conversation_id"`
	Role           string    `json:"role"` // 'user', 'assistant'
	Content        string    `json:"content"`
	TokenCount     int       `json:"token_count"`
	CreatedAt      time.Time `json:"created_at"`
}

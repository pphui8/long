package repository

import (
	"context"
	"database/sql"

	"github.com/pphui8/long/domain"
)

type LLMRepository interface {
	CreateConversation(ctx context.Context, username string, title string) (int, error)
	GetConversation(ctx context.Context, id int) (*domain.Conversation, error)
	GetConversationsByUsername(ctx context.Context, username string) ([]domain.Conversation, error)
	SaveMessage(ctx context.Context, msg domain.Message) error
	GetMessagesByConversationID(ctx context.Context, conversationID int) ([]domain.Message, error)
	DeleteConversation(ctx context.Context, id int) error
}

type llmRepository struct {
	db *sql.DB
}

func NewLLMRepository(db *sql.DB) LLMRepository {
	return &llmRepository{db: db}
}

func (r *llmRepository) CreateConversation(ctx context.Context, username string, title string) (int, error) {
	var id int
	query := "INSERT INTO conversations (username, title) VALUES ($1, $2) RETURNING id"
	err := r.db.QueryRowContext(ctx, query, username, title).Scan(&id)
	return id, err
}

func (r *llmRepository) GetConversation(ctx context.Context, id int) (*domain.Conversation, error) {
	var conv domain.Conversation
	query := "SELECT id, username, title, COALESCE(summary, ''), created_at FROM conversations WHERE id = $1"
	err := r.db.QueryRowContext(ctx, query, id).Scan(&conv.ID, &conv.Username, &conv.Title, &conv.Summary, &conv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

func (r *llmRepository) GetConversationsByUsername(ctx context.Context, username string) ([]domain.Conversation, error) {
	query := "SELECT id, username, title, COALESCE(summary, ''), created_at FROM conversations WHERE username = $1 ORDER BY created_at DESC"
	rows, err := r.db.QueryContext(ctx, query, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conversations := []domain.Conversation{}
	for rows.Next() {
		var conv domain.Conversation
		if err := rows.Scan(&conv.ID, &conv.Username, &conv.Title, &conv.Summary, &conv.CreatedAt); err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}
	return conversations, nil
}

func (r *llmRepository) SaveMessage(ctx context.Context, msg domain.Message) error {
	query := "INSERT INTO messages (conversation_id, role, content, token_count) VALUES ($1, $2, $3, $4)"
	_, err := r.db.ExecContext(ctx, query, msg.ConversationID, msg.Role, msg.Content, msg.TokenCount)
	return err
}

func (r *llmRepository) GetMessagesByConversationID(ctx context.Context, conversationID int) ([]domain.Message, error) {
	query := "SELECT id, conversation_id, role, content, COALESCE(token_count, 0), created_at FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC"
	rows, err := r.db.QueryContext(ctx, query, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := []domain.Message{}
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.TokenCount, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (r *llmRepository) DeleteConversation(ctx context.Context, id int) error {
	query := "DELETE FROM conversations WHERE id = $1"
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

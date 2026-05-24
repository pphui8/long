package repository

import (
	"context"
	"database/sql"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

type LLMRepository interface {
	WithTx(ctx context.Context, fn func(LLMRepository) error) error
	CreateConversation(ctx context.Context, username string, title string) (int, error)
	GetConversation(ctx context.Context, id int) (*domain.Conversation, error)
	GetConversationsByUsername(ctx context.Context, username string) ([]domain.Conversation, error)
	SaveMessage(ctx context.Context, msg domain.Message) error
	GetMessagesByConversationID(ctx context.Context, conversationID int) ([]domain.Message, error)
	GetMessagesByConversationIDAfterID(ctx context.Context, conversationID int, afterMessageID int) ([]domain.Message, error)
	GetLatestConversationSummary(ctx context.Context, conversationID int) (*domain.ConversationSummary, error)
	SaveConversationSummary(ctx context.Context, summary domain.ConversationSummary) error
	DeleteConversation(ctx context.Context, id int) error
}

type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type llmRepository struct {
	db  *sql.DB
	q   dbExecutor
	log *zap.Logger
}

func NewLLMRepository(db *sql.DB, log *zap.Logger) LLMRepository {
	return &llmRepository{db: db, q: db, log: log}
}

func (r *llmRepository) WithTx(ctx context.Context, fn func(LLMRepository) error) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to begin LLM transaction", zap.Error(err))
		return err
	}

	txRepo := &llmRepository{db: r.db, q: tx, log: r.log}
	if err := fn(txRepo); err != nil {
		_ = tx.Rollback()
		logger.WithContext(r.log, ctx).Warn("DB: Rolled back LLM transaction", zap.Error(err))
		return err
	}

	if err := tx.Commit(); err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to commit LLM transaction", zap.Error(err))
		return err
	}
	return nil
}

func (r *llmRepository) CreateConversation(ctx context.Context, username string, title string) (int, error) {
	var id int
	query := "INSERT INTO conversations (username, title) VALUES ($1, $2) RETURNING id"
	err := r.q.QueryRowContext(ctx, query, username, title).Scan(&id)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to create conversation", zap.String("username", username), zap.Error(err))
		return id, err
	}
	logger.WithContext(r.log, ctx).Info("DB: Created conversation", zap.String("username", username), zap.Int("conversation_id", id))
	return id, err
}

func (r *llmRepository) GetConversation(ctx context.Context, id int) (*domain.Conversation, error) {
	var conv domain.Conversation
	query := "SELECT id, username, title, COALESCE(summary, ''), created_at, COALESCE(last_message_at, created_at) FROM conversations WHERE id = $1"
	err := r.q.QueryRowContext(ctx, query, id).Scan(&conv.ID, &conv.Username, &conv.Title, &conv.Summary, &conv.CreatedAt, &conv.LastMessageAt)
	if err != nil {
		if err != sql.ErrNoRows {
			logger.WithContext(r.log, ctx).Error("DB: Failed to get conversation", zap.Int("conversation_id", id), zap.Error(err))
		}
		return nil, err
	}
	return &conv, nil
}

func (r *llmRepository) GetConversationsByUsername(ctx context.Context, username string) ([]domain.Conversation, error) {
	query := "SELECT id, username, title, COALESCE(summary, ''), created_at, COALESCE(last_message_at, created_at) FROM conversations WHERE username = $1 ORDER BY last_message_at DESC NULLS LAST, created_at DESC"
	rows, err := r.q.QueryContext(ctx, query, username)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to list conversations", zap.String("username", username), zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	conversations := []domain.Conversation{}
	for rows.Next() {
		var conv domain.Conversation
		if err := rows.Scan(&conv.ID, &conv.Username, &conv.Title, &conv.Summary, &conv.CreatedAt, &conv.LastMessageAt); err != nil {
			logger.WithContext(r.log, ctx).Error("DB: Failed to scan conversation", zap.String("username", username), zap.Error(err))
			return nil, err
		}
		conversations = append(conversations, conv)
	}
	if err := rows.Err(); err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Conversation rows failed", zap.String("username", username), zap.Error(err))
		return nil, err
	}
	return conversations, nil
}

func (r *llmRepository) SaveMessage(ctx context.Context, msg domain.Message) error {
	query := `
WITH inserted AS (
	INSERT INTO messages (conversation_id, role, content, token_count)
	VALUES ($1, $2, $3, $4)
	RETURNING conversation_id, created_at
)
UPDATE conversations
SET last_message_at = inserted.created_at
FROM inserted
WHERE conversations.id = inserted.conversation_id`
	_, err := r.q.ExecContext(ctx, query, msg.ConversationID, msg.Role, msg.Content, msg.TokenCount)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to save message", zap.Int("conversation_id", msg.ConversationID), zap.String("role", msg.Role), zap.Error(err))
		return err
	}
	logger.WithContext(r.log, ctx).Info("DB: Saved message", zap.Int("conversation_id", msg.ConversationID), zap.String("role", msg.Role))
	return err
}

func (r *llmRepository) GetMessagesByConversationID(ctx context.Context, conversationID int) ([]domain.Message, error) {
	query := "SELECT id, conversation_id, role, content, COALESCE(token_count, 0), created_at FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC"
	rows, err := r.q.QueryContext(ctx, query, conversationID)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to get messages", zap.Int("conversation_id", conversationID), zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	messages := []domain.Message{}
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.TokenCount, &msg.CreatedAt); err != nil {
			logger.WithContext(r.log, ctx).Error("DB: Failed to scan message", zap.Int("conversation_id", conversationID), zap.Error(err))
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Message rows failed", zap.Int("conversation_id", conversationID), zap.Error(err))
		return nil, err
	}
	return messages, nil
}

func (r *llmRepository) GetMessagesByConversationIDAfterID(ctx context.Context, conversationID int, afterMessageID int) ([]domain.Message, error) {
	query := "SELECT id, conversation_id, role, content, COALESCE(token_count, 0), created_at FROM messages WHERE conversation_id = $1 AND id > $2 ORDER BY created_at ASC"
	rows, err := r.q.QueryContext(ctx, query, conversationID, afterMessageID)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to get messages after checkpoint", zap.Int("conversation_id", conversationID), zap.Int("after_message_id", afterMessageID), zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	messages := []domain.Message{}
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.TokenCount, &msg.CreatedAt); err != nil {
			logger.WithContext(r.log, ctx).Error("DB: Failed to scan checkpoint messages", zap.Int("conversation_id", conversationID), zap.Error(err))
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Checkpoint message rows failed", zap.Int("conversation_id", conversationID), zap.Error(err))
		return nil, err
	}
	return messages, nil
}

func (r *llmRepository) GetLatestConversationSummary(ctx context.Context, conversationID int) (*domain.ConversationSummary, error) {
	var summary domain.ConversationSummary
	query := `
SELECT id, conversation_id, summary, summarized_through_message_id, summarized_token_count, created_at, updated_at
FROM conversation_summaries
WHERE conversation_id = $1
ORDER BY summarized_through_message_id DESC
LIMIT 1`
	err := r.q.QueryRowContext(ctx, query, conversationID).Scan(
		&summary.ID,
		&summary.ConversationID,
		&summary.Summary,
		&summary.SummarizedThroughMessageID,
		&summary.SummarizedTokenCount,
		&summary.CreatedAt,
		&summary.UpdatedAt,
	)
	if err != nil {
		if err != sql.ErrNoRows {
			logger.WithContext(r.log, ctx).Error("DB: Failed to get latest conversation summary", zap.Int("conversation_id", conversationID), zap.Error(err))
		}
		return nil, err
	}
	return &summary, nil
}

func (r *llmRepository) SaveConversationSummary(ctx context.Context, summary domain.ConversationSummary) error {
	query := `
WITH inserted AS (
	INSERT INTO conversation_summaries (conversation_id, summary, summarized_through_message_id, summarized_token_count)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (conversation_id, summarized_through_message_id)
	DO UPDATE SET
		summary = EXCLUDED.summary,
		summarized_token_count = EXCLUDED.summarized_token_count,
		updated_at = CURRENT_TIMESTAMP
	RETURNING conversation_id, summary
)
UPDATE conversations
SET summary = inserted.summary
FROM inserted
WHERE conversations.id = inserted.conversation_id`
	_, err := r.q.ExecContext(ctx, query, summary.ConversationID, summary.Summary, summary.SummarizedThroughMessageID, summary.SummarizedTokenCount)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to save conversation summary", zap.Int("conversation_id", summary.ConversationID), zap.Int("summarized_through_message_id", summary.SummarizedThroughMessageID), zap.Error(err))
		return err
	}
	logger.WithContext(r.log, ctx).Info("DB: Saved conversation summary", zap.Int("conversation_id", summary.ConversationID), zap.Int("summarized_through_message_id", summary.SummarizedThroughMessageID))
	return nil
}

func (r *llmRepository) DeleteConversation(ctx context.Context, id int) error {
	query := "DELETE FROM conversations WHERE id = $1"
	_, err := r.q.ExecContext(ctx, query, id)
	if err != nil {
		logger.WithContext(r.log, ctx).Error("DB: Failed to delete conversation", zap.Int("conversation_id", id), zap.Error(err))
		return err
	}
	logger.WithContext(r.log, ctx).Info("DB: Deleted conversation", zap.Int("conversation_id", id))
	return err
}

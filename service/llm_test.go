package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/repository"
)

type summaryTestProvider struct {
	chunks []string
}

func (p summaryTestProvider) Name() string {
	return "test"
}

func (p summaryTestProvider) Stream(ctx context.Context, history []domain.Message, onChunk func(string) error) error {
	for _, chunk := range p.chunks {
		if err := onChunk(chunk); err != nil {
			return err
		}
	}
	return nil
}

type summaryTestRepo struct {
	messages     []domain.Message
	latest       *domain.ConversationSummary
	savedSummary *domain.ConversationSummary
}

func (r *summaryTestRepo) WithTx(ctx context.Context, fn func(repository.LLMRepository) error) error {
	return fn(r)
}

func (r *summaryTestRepo) CreateConversation(ctx context.Context, username string, title string) (int, error) {
	return 0, nil
}

func (r *summaryTestRepo) GetConversation(ctx context.Context, id int) (*domain.Conversation, error) {
	return nil, sql.ErrNoRows
}

func (r *summaryTestRepo) GetConversationsByUsername(ctx context.Context, username string) ([]domain.Conversation, error) {
	return nil, nil
}

func (r *summaryTestRepo) SaveMessage(ctx context.Context, msg domain.Message) error {
	return nil
}

func (r *summaryTestRepo) GetMessagesByConversationID(ctx context.Context, conversationID int) ([]domain.Message, error) {
	return r.messages, nil
}

func (r *summaryTestRepo) GetMessagesByConversationIDAfterID(ctx context.Context, conversationID int, afterMessageID int) ([]domain.Message, error) {
	messages := make([]domain.Message, 0, len(r.messages))
	for _, msg := range r.messages {
		if msg.ID > afterMessageID {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

func (r *summaryTestRepo) GetLatestConversationSummary(ctx context.Context, conversationID int) (*domain.ConversationSummary, error) {
	if r.latest == nil {
		return nil, sql.ErrNoRows
	}
	return r.latest, nil
}

func (r *summaryTestRepo) SaveConversationSummary(ctx context.Context, summary domain.ConversationSummary) error {
	summary.CreatedAt = time.Time{}
	summary.UpdatedAt = time.Time{}
	r.savedSummary = &summary
	return nil
}

func (r *summaryTestRepo) DeleteConversation(ctx context.Context, id int) error {
	return nil
}

func TestSummarizeConversationIfNeededSummarizesAtZeroInterval(t *testing.T) {
	repo := &summaryTestRepo{
		messages: []domain.Message{
			{ID: 10, ConversationID: 7, Role: "user", Content: "hello", TokenCount: 2},
			{ID: 11, ConversationID: 7, Role: "assistant", Content: "hi", TokenCount: 1},
		},
	}
	svc := &llmService{repo: repo}

	err := svc.summarizeConversationIfNeeded(context.Background(), summaryTestProvider{chunks: []string{"new ", "summary"}}, 7)
	if err != nil {
		t.Fatalf("summarizeConversationIfNeeded returned error: %v", err)
	}
	if repo.savedSummary == nil {
		t.Fatal("expected summary checkpoint to be saved")
	}
	if repo.savedSummary.Summary != "new summary" {
		t.Fatalf("summary = %q, want %q", repo.savedSummary.Summary, "new summary")
	}
	if repo.savedSummary.SummarizedThroughMessageID != 11 {
		t.Fatalf("summarized through message ID = %d, want 11", repo.savedSummary.SummarizedThroughMessageID)
	}
	if repo.savedSummary.SummarizedTokenCount != 3 {
		t.Fatalf("summarized token count = %d, want 3", repo.savedSummary.SummarizedTokenCount)
	}
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/repository"
	"go.uber.org/zap"
)

type LLMService interface {
	StreamPrompt(ctx context.Context, username string, req domain.LLMRequest, onChunk func(string) error) (int, error)
	GetConversations(ctx context.Context, username string) ([]domain.Conversation, error)
	GetMessages(ctx context.Context, username string, conversationID int) ([]domain.Message, error)
	DeleteConversation(ctx context.Context, username string, conversationID int) error
	ValidateConversationAccess(ctx context.Context, username string, conversationID int) error
}

type llmService struct {
	repo     repository.LLMRepository
	provider ChatProvider
	log      *zap.Logger
}

func NewLLMService(repo repository.LLMRepository, provider ChatProvider, log *zap.Logger) (LLMService, error) {
	if provider == nil {
		return nil, errors.New("chat provider is required")
	}

	return &llmService{repo: repo, provider: provider, log: log}, nil
}

func (s *llmService) GetConversations(ctx context.Context, username string) ([]domain.Conversation, error) {
	return s.repo.GetConversationsByUsername(ctx, username)
}

func (s *llmService) GetMessages(ctx context.Context, username string, conversationID int) ([]domain.Message, error) {
	if _, err := s.getOwnedConversation(ctx, username, conversationID); err != nil {
		return nil, err
	}

	return s.repo.GetMessagesByConversationID(ctx, conversationID)
}

func (s *llmService) DeleteConversation(ctx context.Context, username string, conversationID int) error {
	if _, err := s.getOwnedConversation(ctx, username, conversationID); err != nil {
		return err
	}

	return s.repo.DeleteConversation(ctx, conversationID)
}

func (s *llmService) ValidateConversationAccess(ctx context.Context, username string, conversationID int) error {
	_, err := s.getOwnedConversation(ctx, username, conversationID)
	return err
}

func (s *llmService) StreamPrompt(ctx context.Context, username string, req domain.LLMRequest, onChunk func(string) error) (int, error) {
	if err := validateLLMRequest(req); err != nil {
		return 0, err
	}

	log := logger.WithContext(s.log, ctx)
	var conversationID int
	err := s.repo.WithTx(ctx, func(txRepo repository.LLMRepository) error {
		var err error

		if req.ConversationID == nil {
			// Start a new conversation
			title := req.Prompt
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			conversationID, err = txRepo.CreateConversation(ctx, username, title)
			if err != nil {
				return fmt.Errorf("failed to create conversation: %w", err)
			}
		} else {
			conversationID = *req.ConversationID
			if _, err := getOwnedConversation(ctx, txRepo, username, conversationID); err != nil {
				return err
			}
		}

		// Save user message
		userMsg := domain.Message{
			ConversationID: conversationID,
			Role:           "user",
			Content:        req.Prompt,
		}
		if err := txRepo.SaveMessage(ctx, userMsg); err != nil {
			return fmt.Errorf("failed to save user message: %w", err)
		}

		// Get conversation history
		history, err := txRepo.GetMessagesByConversationID(ctx, conversationID)
		if err != nil {
			return fmt.Errorf("failed to get history: %w", err)
		}

		var fullResponse string
		log.Info("LLM: Starting provider stream", zap.String("username", username), zap.Int("conversation_id", conversationID), zap.String("provider", s.provider.Name()), zap.Int("history_messages", len(history)))
		if err := s.provider.Stream(ctx, history, func(chunk string) error {
			fullResponse += chunk
			return onChunk(chunk)
		}); err != nil {
			log.Error("LLM: Provider stream failed", zap.String("username", username), zap.Int("conversation_id", conversationID), zap.String("provider", s.provider.Name()), zap.Error(err))
			return fmt.Errorf("failed to generate streaming content: %w", err)
		}
		log.Info("LLM: Provider stream completed", zap.String("username", username), zap.Int("conversation_id", conversationID), zap.String("provider", s.provider.Name()), zap.Int("response_bytes", len(fullResponse)))

		// Save assistant message
		assistantMsg := domain.Message{
			ConversationID: conversationID,
			Role:           "assistant",
			Content:        fullResponse,
		}
		if err := txRepo.SaveMessage(ctx, assistantMsg); err != nil {
			return fmt.Errorf("failed to save assistant message: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, err
	}

	return conversationID, nil
}

func (s *llmService) getOwnedConversation(ctx context.Context, username string, conversationID int) (*domain.Conversation, error) {
	return getOwnedConversation(ctx, s.repo, username, conversationID)
}

func getOwnedConversation(ctx context.Context, repo repository.LLMRepository, username string, conversationID int) (*domain.Conversation, error) {
	if conversationID <= 0 {
		return nil, fmt.Errorf("conversation id must be positive: %w", domain.ErrValidation)
	}

	conv, err := repo.GetConversation(ctx, conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("conversation not found: %w", domain.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation: %w", err)
	}
	if conv.Username != username {
		return nil, fmt.Errorf("unauthorized access to conversation: %w", domain.ErrForbidden)
	}

	return conv, nil
}

func validateLLMRequest(req domain.LLMRequest) error {
	if strings.TrimSpace(req.Prompt) == "" {
		return fmt.Errorf("prompt is required: %w", domain.ErrValidation)
	}
	if req.ConversationID != nil && *req.ConversationID <= 0 {
		return fmt.Errorf("conversation id must be positive: %w", domain.ErrValidation)
	}
	return nil
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/repository"
)

type LLMService interface {
	ProcessPrompt(ctx context.Context, username string, req domain.LLMRequest) (domain.LLMResponse, error)
	StreamPrompt(ctx context.Context, username string, req domain.LLMRequest, onChunk func(string) error) (int, error)
	GetConversations(ctx context.Context, username string) ([]domain.Conversation, error)
	GetMessages(ctx context.Context, username string, conversationID int) ([]domain.Message, error)
	DeleteConversation(ctx context.Context, username string, conversationID int) error
	ValidateConversationAccess(ctx context.Context, username string, conversationID int) error
}

type llmService struct {
	repo     repository.LLMRepository
	provider ChatProvider
}

func NewLLMService(repo repository.LLMRepository, provider ChatProvider) (LLMService, error) {
	if provider == nil {
		return nil, errors.New("chat provider is required")
	}

	return &llmService{repo: repo, provider: provider}, nil
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

func (s *llmService) ProcessPrompt(ctx context.Context, username string, req domain.LLMRequest) (domain.LLMResponse, error) {
	if err := validateLLMRequest(req); err != nil {
		return domain.LLMResponse{}, err
	}

	var conversationID int
	var err error

	if req.ConversationID == nil {
		// Start a new conversation
		title := req.Prompt
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		conversationID, err = s.repo.CreateConversation(ctx, username, title)
		if err != nil {
			return domain.LLMResponse{}, fmt.Errorf("failed to create conversation: %w", err)
		}
	} else {
		conversationID = *req.ConversationID
		if _, err := s.getOwnedConversation(ctx, username, conversationID); err != nil {
			return domain.LLMResponse{}, err
		}
	}

	// Save user message
	userMsg := domain.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Prompt,
	}
	if err := s.repo.SaveMessage(ctx, userMsg); err != nil {
		return domain.LLMResponse{}, fmt.Errorf("failed to save user message: %w", err)
	}

	// Get conversation history
	history, err := s.repo.GetMessagesByConversationID(ctx, conversationID)
	if err != nil {
		return domain.LLMResponse{}, fmt.Errorf("failed to get history: %w", err)
	}

	assistantText, err := s.provider.Generate(ctx, history)
	if err != nil {
		return domain.LLMResponse{}, fmt.Errorf("failed to generate content: %w", err)
	}

	// Save assistant message
	assistantMsg := domain.Message{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        assistantText,
	}
	if err := s.repo.SaveMessage(ctx, assistantMsg); err != nil {
		return domain.LLMResponse{}, fmt.Errorf("failed to save assistant message: %w", err)
	}

	return domain.LLMResponse{
		ConversationID: conversationID,
		Text:           assistantText,
	}, nil
}

func (s *llmService) StreamPrompt(ctx context.Context, username string, req domain.LLMRequest, onChunk func(string) error) (int, error) {
	if err := validateLLMRequest(req); err != nil {
		return 0, err
	}

	var conversationID int
	var err error

	if req.ConversationID == nil {
		// Start a new conversation
		title := req.Prompt
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		conversationID, err = s.repo.CreateConversation(ctx, username, title)
		if err != nil {
			return 0, fmt.Errorf("failed to create conversation: %w", err)
		}
	} else {
		conversationID = *req.ConversationID
		if _, err := s.getOwnedConversation(ctx, username, conversationID); err != nil {
			return 0, err
		}
	}

	// Save user message
	userMsg := domain.Message{
		ConversationID: conversationID,
		Role:           "user",
		Content:        req.Prompt,
	}
	if err := s.repo.SaveMessage(ctx, userMsg); err != nil {
		return 0, fmt.Errorf("failed to save user message: %w", err)
	}

	// Get conversation history
	history, err := s.repo.GetMessagesByConversationID(ctx, conversationID)
	if err != nil {
		return 0, fmt.Errorf("failed to get history: %w", err)
	}

	var fullResponse string
	if err := s.provider.Stream(ctx, history, func(chunk string) error {
		fullResponse += chunk
		return onChunk(chunk)
	}); err != nil {
		return 0, fmt.Errorf("failed to generate streaming content: %w", err)
	}

	// Save assistant message
	assistantMsg := domain.Message{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        fullResponse,
	}
	if err := s.repo.SaveMessage(ctx, assistantMsg); err != nil {
		return 0, fmt.Errorf("failed to save assistant message: %w", err)
	}

	return conversationID, nil
}

func (s *llmService) getOwnedConversation(ctx context.Context, username string, conversationID int) (*domain.Conversation, error) {
	if conversationID <= 0 {
		return nil, fmt.Errorf("conversation id must be positive: %w", domain.ErrValidation)
	}

	conv, err := s.repo.GetConversation(ctx, conversationID)
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

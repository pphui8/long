package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/llmengine"
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/repository"
	"go.uber.org/zap"
)

const (
	conversationSummaryTokenInterval = 0
)

type LLMService interface {
	StreamPrompt(ctx context.Context, username string, req domain.LLMRequest, onChunk func(string) error) (int, error)
	GetConversations(ctx context.Context, username string) ([]domain.Conversation, error)
	GetMessages(ctx context.Context, username string, conversationID int) ([]domain.Message, error)
	DeleteConversation(ctx context.Context, username string, conversationID int) error
	ValidateConversationAccess(ctx context.Context, username string, conversationID int) error
	ValidateModel(model string) error
}

type llmService struct {
	repo      repository.LLMRepository
	providers map[string]ChatProvider
	engines   map[string]*llmengine.Engine
	log       *zap.Logger
}

func NewLLMService(repo repository.LLMRepository, provider ChatProvider, log *zap.Logger) (LLMService, error) {
	if provider == nil {
		return nil, errors.New("chat provider is required")
	}

	return NewLLMServiceWithProviders(repo, []ChatProvider{provider}, log)
}

func NewLLMServiceWithProviders(repo repository.LLMRepository, providers []ChatProvider, log *zap.Logger) (LLMService, error) {
	if len(providers) == 0 {
		return nil, errors.New("at least one chat provider is required")
	}

	providerByModel := make(map[string]ChatProvider, len(providers))
	engineByModel := make(map[string]*llmengine.Engine, len(providers))
	for _, provider := range providers {
		if provider == nil {
			return nil, errors.New("chat provider is required")
		}
		model := strings.TrimSpace(provider.Name())
		if model == "" {
			return nil, errors.New("chat provider name is required")
		}
		if _, exists := providerByModel[model]; exists {
			return nil, fmt.Errorf("duplicate chat provider model %q", model)
		}

		engine, err := llmengine.New(provider)
		if err != nil {
			return nil, err
		}
		providerByModel[model] = provider
		engineByModel[model] = engine
	}

	return &llmService{repo: repo, providers: providerByModel, engines: engineByModel, log: log}, nil
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

func (s *llmService) ValidateModel(model string) error {
	_, _, err := s.providerForModel(strings.TrimSpace(model))
	return err
}

func (s *llmService) StreamPrompt(ctx context.Context, username string, req domain.LLMRequest, onChunk func(string) error) (int, error) {
	if err := validateLLMRequest(req); err != nil {
		return 0, err
	}

	log := logger.WithContext(s.log, ctx)
	model := strings.TrimSpace(req.Model)
	provider, engine, err := s.providerForModel(model)
	if err != nil {
		return 0, err
	}

	var conversationID int
	var history []domain.Message
	if err := s.repo.WithTx(ctx, func(txRepo repository.LLMRepository) error {
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
			TokenCount:     estimateTokenCount(req.Prompt),
		}
		if err := txRepo.SaveMessage(ctx, userMsg); err != nil {
			return fmt.Errorf("failed to save user message: %w", err)
		}

		// Get conversation history as the latest summary checkpoint plus only new raw messages.
		history, err = buildHistory(ctx, txRepo, conversationID)
		if err != nil {
			return fmt.Errorf("failed to get history: %w", err)
		}

		return nil
	}); err != nil {
		return 0, err
	}

	log.Info("LLM: Starting provider stream", zap.String("username", username), zap.Int("conversation_id", conversationID), zap.String("model", model), zap.String("provider", provider.Name()), zap.Int("history_messages", len(history)))
	result, err := engine.Stream(ctx, llmengine.StreamRequest{
		Username:       username,
		ConversationID: conversationID,
		History:        history,
	}, onChunk)
	if err != nil {
		log.Error("LLM: Provider stream failed", zap.String("username", username), zap.Int("conversation_id", conversationID), zap.String("model", model), zap.String("provider", provider.Name()), zap.Error(err))
		return 0, fmt.Errorf("failed to generate streaming content: %w", err)
	}
	log.Info("LLM: Provider stream completed", zap.String("username", username), zap.Int("conversation_id", conversationID), zap.String("model", model), zap.String("provider", provider.Name()), zap.Int("response_bytes", len(result.Content)))

	// Save assistant message after the provider call so the DB transaction is not held during streaming.
	assistantMsg := domain.Message{
		ConversationID: conversationID,
		Role:           "assistant",
		Content:        result.Content,
		TokenCount:     estimateTokenCount(result.Content),
	}
	if err := s.repo.SaveMessage(ctx, assistantMsg); err != nil {
		return 0, fmt.Errorf("failed to save assistant message: %w", err)
	}
	if err := s.summarizeConversationIfNeeded(ctx, provider, conversationID); err != nil {
		log.Warn("LLM: Conversation summarization failed", zap.Int("conversation_id", conversationID), zap.String("model", model), zap.String("provider", provider.Name()), zap.Error(err))
	}

	return conversationID, nil
}

func buildHistory(ctx context.Context, repo repository.LLMRepository, conversationID int) ([]domain.Message, error) {
	latestSummary, err := repo.GetLatestConversationSummary(ctx, conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		return repo.GetMessagesByConversationID(ctx, conversationID)
	}
	if err != nil {
		return nil, err
	}

	messages, err := repo.GetMessagesByConversationIDAfterID(ctx, conversationID, latestSummary.SummarizedThroughMessageID)
	if err != nil {
		return nil, err
	}

	history := make([]domain.Message, 0, len(messages)+1)
	history = append(history, domain.Message{
		ConversationID: conversationID,
		Role:           "system",
		Content:        "Conversation summary so far:\n" + latestSummary.Summary,
		TokenCount:     estimateTokenCount(latestSummary.Summary),
	})
	history = append(history, messages...)
	return history, nil
}

func (s *llmService) summarizeConversationIfNeeded(ctx context.Context, provider ChatProvider, conversationID int) error {
	latestSummary, err := s.repo.GetLatestConversationSummary(ctx, conversationID)
	if errors.Is(err, sql.ErrNoRows) {
		latestSummary = nil
	} else if err != nil {
		return err
	}

	afterMessageID := 0
	cumulativeTokenCount := 0
	previousSummary := ""
	if latestSummary != nil {
		afterMessageID = latestSummary.SummarizedThroughMessageID
		cumulativeTokenCount = latestSummary.SummarizedTokenCount
		previousSummary = latestSummary.Summary
	}

	messages, err := s.repo.GetMessagesByConversationIDAfterID(ctx, conversationID, afterMessageID)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	newTokenCount := tokenCount(messages)
	if newTokenCount < conversationSummaryTokenInterval {
		return nil
	}

	summary, err := generateConversationSummary(ctx, provider, previousSummary, messages)
	if err != nil {
		return err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return errors.New("empty conversation summary")
	}

	lastMessage := messages[len(messages)-1]
	return s.repo.SaveConversationSummary(ctx, domain.ConversationSummary{
		ConversationID:             conversationID,
		Summary:                    summary,
		SummarizedThroughMessageID: lastMessage.ID,
		SummarizedTokenCount:       cumulativeTokenCount + newTokenCount,
	})
}

func generateConversationSummary(ctx context.Context, provider ChatProvider, previousSummary string, messages []domain.Message) (string, error) {
	var b strings.Builder
	if strings.TrimSpace(previousSummary) != "" {
		b.WriteString("Previous summary:\n")
		b.WriteString(previousSummary)
		b.WriteString("\n\n")
	}
	b.WriteString("New conversation messages:\n")
	for _, msg := range messages {
		b.WriteString(strings.ToUpper(msg.Role))
		b.WriteString(": ")
		b.WriteString(msg.Content)
		b.WriteString("\n\n")
	}

	prompt := []domain.Message{
		{
			Role: "system",
			Content: strings.Join([]string{
				"You summarize long chat conversations for future LLM context.",
				"Create one concise but complete rolling summary that preserves decisions, facts, constraints, user preferences, unresolved tasks, and important code or data references.",
				"Merge the previous summary with the new messages. Do not mention that you are summarizing.",
			}, " "),
		},
		{
			Role:    "user",
			Content: b.String(),
		},
	}

	var summary strings.Builder
	if err := provider.Stream(ctx, prompt, func(chunk string) error {
		_, err := summary.WriteString(chunk)
		return err
	}); err != nil {
		return "", err
	}
	return summary.String(), nil
}

func tokenCount(messages []domain.Message) int {
	total := 0
	for _, msg := range messages {
		if msg.TokenCount > 0 {
			total += msg.TokenCount
			continue
		}
		total += estimateTokenCount(msg.Content)
	}
	return total
}

func estimateTokenCount(content string) int {
	if content == "" {
		return 0
	}
	runes := utf8.RuneCountInString(content)
	tokens := runes / 4
	if runes%4 != 0 {
		tokens++
	}
	if tokens == 0 {
		return 1
	}
	return tokens
}

func (s *llmService) getOwnedConversation(ctx context.Context, username string, conversationID int) (*domain.Conversation, error) {
	return getOwnedConversation(ctx, s.repo, username, conversationID)
}

func (s *llmService) providerForModel(model string) (ChatProvider, *llmengine.Engine, error) {
	if model == "" {
		return nil, nil, fmt.Errorf("model is required: %w", domain.ErrValidation)
	}
	provider, ok := s.providers[model]
	if !ok {
		return nil, nil, fmt.Errorf("unsupported model %q: %w", model, domain.ErrValidation)
	}
	engine, ok := s.engines[model]
	if !ok {
		return nil, nil, fmt.Errorf("model %q is not configured: %w", model, domain.ErrValidation)
	}
	return provider, engine, nil
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
	if strings.TrimSpace(req.Model) == "" {
		return fmt.Errorf("model is required: %w", domain.ErrValidation)
	}
	if req.ConversationID != nil && *req.ConversationID <= 0 {
		return fmt.Errorf("conversation id must be positive: %w", domain.ErrValidation)
	}
	return nil
}

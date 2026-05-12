package service

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/repository"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

type LLMService interface {
	ProcessPrompt(ctx context.Context, username string, req domain.LLMRequest) (domain.LLMResponse, error)
	GetConversations(ctx context.Context, username string) ([]domain.Conversation, error)
}

type llmService struct {
	repo repository.LLMRepository
	llm  *googleai.GoogleAI
}

func NewLLMService(repo repository.LLMRepository) (LLMService, error) {
	apiKey := os.Getenv("GEMINI_API")
	if apiKey == "" {
		return nil, errors.New("GEMINI_API environment variable not set")
	}

	ctx := context.Background()
	llm, err := googleai.New(ctx, googleai.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create googleai llm: %w", err)
	}

	return &llmService{
		repo: repo,
		llm:  llm,
	}, nil
}

func (s *llmService) GetConversations(ctx context.Context, username string) ([]domain.Conversation, error) {
	return s.repo.GetConversationsByUsername(ctx, username)
}

func (s *llmService) ProcessPrompt(ctx context.Context, username string, req domain.LLMRequest) (domain.LLMResponse, error) {
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
		// Verify conversation ownership
		conv, err := s.repo.GetConversation(ctx, conversationID)
		if err != nil {
			return domain.LLMResponse{}, fmt.Errorf("failed to get conversation: %w", err)
		}
		if conv.Username != username {
			return domain.LLMResponse{}, errors.New("unauthorized access to conversation")
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

	var content []llms.MessageContent
	for _, msg := range history {
		role := llms.ChatMessageTypeHuman
		if msg.Role == "assistant" {
			role = llms.ChatMessageTypeAI
		} else if msg.Role == "system" {
			role = llms.ChatMessageTypeSystem
		}
		content = append(content, llms.TextParts(role, msg.Content))
	}

	// Call LLM
	resp, err := s.llm.GenerateContent(ctx, content)
	if err != nil {
		return domain.LLMResponse{}, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Choices) == 0 {
		return domain.LLMResponse{}, errors.New("no response from LLM")
	}

	assistantText := resp.Choices[0].Content

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

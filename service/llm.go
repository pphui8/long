package service

import (
	"context"
	"github.com/pphui8/long/domain"
)

type LLMService interface {
	ProcessPrompt(ctx context.Context, req domain.LLMRequest) (domain.LLMResponse, error)
}

type llmService struct {
	// dependencies like repository or AI client
}

func NewLLMService() LLMService {
	return &llmService{}
}

func (s *llmService) ProcessPrompt(ctx context.Context, req domain.LLMRequest) (domain.LLMResponse, error) {
	// Placeholder for real business flow
	return domain.LLMResponse{Text: "LLM response placeholder"}, nil
}

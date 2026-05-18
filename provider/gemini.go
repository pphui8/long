package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/service"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/googleai"
)

const GeminiProviderName = "gemini"

type GeminiProvider struct {
	llm *googleai.GoogleAI
}

func NewGeminiProvider(ctx context.Context, cfg service.ProviderConfig) (*GeminiProvider, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("gemini API key is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("gemini model is required")
	}

	llm, err := googleai.New(ctx, googleai.WithAPIKey(cfg.APIKey), googleai.WithDefaultModel(cfg.Model))
	if err != nil {
		return nil, fmt.Errorf("failed to create googleai llm: %w", err)
	}

	return &GeminiProvider{llm: llm}, nil
}

func (p *GeminiProvider) Name() string {
	return GeminiProviderName
}

func (p *GeminiProvider) Generate(ctx context.Context, history []domain.Message) (string, error) {
	resp, err := p.llm.GenerateContent(ctx, toMessageContent(history))
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("no response from LLM")
	}
	return resp.Choices[0].Content, nil
}

func (p *GeminiProvider) Stream(ctx context.Context, history []domain.Message, onChunk func(string) error) error {
	_, err := p.llm.GenerateContent(ctx, toMessageContent(history), llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
		return onChunk(string(chunk))
	}))
	return err
}

func toMessageContent(history []domain.Message) []llms.MessageContent {
	content := make([]llms.MessageContent, 0, len(history))
	for _, msg := range history {
		role := llms.ChatMessageTypeHuman
		if msg.Role == "assistant" {
			role = llms.ChatMessageTypeAI
		} else if msg.Role == "system" {
			role = llms.ChatMessageTypeSystem
		}
		content = append(content, llms.TextParts(role, msg.Content))
	}
	return content
}

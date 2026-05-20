package llmengine

import (
	"context"

	"github.com/pphui8/long/domain"
)

type ChatProvider interface {
	Name() string
	Stream(ctx context.Context, history []domain.Message, onChunk func(string) error) error
}

type Engine struct {
	contextBuilder ContextBuilder
	provider       ChatProvider
	promptBuilder  PromptBuilder
}

type Option func(*Engine)

type StreamRequest struct {
	Username       string
	ConversationID int
	History        []domain.Message
}

type StreamResult struct {
	Content string
}

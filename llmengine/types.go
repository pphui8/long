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
	maxAgentSteps  int
	provider       ChatProvider
	promptBuilder  PromptBuilder
	tools          map[string]Tool
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

type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, input string) (string, error)
}

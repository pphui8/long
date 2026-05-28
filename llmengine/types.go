package llmengine

import (
	"context"

	"github.com/pphui8/long/domain"
	"github.com/tmc/langchaingo/llms"
	lctools "github.com/tmc/langchaingo/tools"
	"go.uber.org/zap"
)

type ChatProvider interface {
	Name() string
	Stream(ctx context.Context, history []domain.Message, onChunk func(string) error) error
}

type ModelProvider interface {
	Model() llms.Model
}

type Engine struct {
	contextBuilder ContextBuilder
	maxAgentSteps  int
	provider       ChatProvider
	promptBuilder  PromptBuilder
	log            *zap.Logger
	tools          []Tool
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

type Tool = lctools.Tool

package llmengine

import (
	"context"
	"errors"
)

func New(provider ChatProvider, opts ...Option) (*Engine, error) {
	if provider == nil {
		return nil, errors.New("chat provider is required")
	}

	engine := &Engine{
		contextBuilder: NewPassthroughContextBuilder(),
		provider:       provider,
		promptBuilder:  NewBasicPromptBuilder(""),
	}
	for _, opt := range opts {
		opt(engine)
	}
	if engine.contextBuilder == nil {
		return nil, errors.New("context builder is required")
	}
	if engine.promptBuilder == nil {
		return nil, errors.New("prompt builder is required")
	}

	return engine, nil
}

func WithPromptBuilder(promptBuilder PromptBuilder) Option {
	return func(engine *Engine) {
		engine.promptBuilder = promptBuilder
	}
}

func WithContextBuilder(contextBuilder ContextBuilder) Option {
	return func(engine *Engine) {
		engine.contextBuilder = contextBuilder
	}
}

func (e *Engine) ProviderName() string {
	return e.provider.Name()
}

func (e *Engine) Stream(ctx context.Context, req StreamRequest, onChunk func(string) error) (*StreamResult, error) {
	modelContext, err := e.contextBuilder.Build(ctx, ContextInput{
		Username:       req.Username,
		ConversationID: req.ConversationID,
		History:        req.History,
	})
	if err != nil {
		return nil, err
	}

	messages, err := e.promptBuilder.Build(PromptInput{
		Username:       req.Username,
		ConversationID: req.ConversationID,
		Context:        modelContext,
	})
	if err != nil {
		return nil, err
	}

	var fullResponse string
	if err := e.provider.Stream(ctx, messages, func(chunk string) error {
		fullResponse += chunk
		if onChunk == nil {
			return nil
		}
		return onChunk(chunk)
	}); err != nil {
		return nil, err
	}

	return &StreamResult{Content: fullResponse}, nil
}

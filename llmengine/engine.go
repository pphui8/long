package llmengine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"go.uber.org/zap"
)

const defaultMaxAgentSteps = 5
const logPreviewLimit = 2000

func New(provider ChatProvider, opts ...Option) (*Engine, error) {
	if provider == nil {
		return nil, errors.New("chat provider is required")
	}

	engine := &Engine{
		contextBuilder: NewPassthroughContextBuilder(),
		log:            zap.NewNop(),
		maxAgentSteps:  defaultMaxAgentSteps,
		provider:       provider,
		promptBuilder:  NewBasicPromptBuilder(""),
		tools:          []Tool{},
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

func WithLogger(log *zap.Logger) Option {
	return func(engine *Engine) {
		if log != nil {
			engine.log = log
		}
	}
}

func WithTools(tools ...Tool) Option {
	return func(engine *Engine) {
		for _, tool := range tools {
			if tool == nil || strings.TrimSpace(tool.Name()) == "" {
				continue
			}
			engine.tools = append(engine.tools, tool)
		}
	}
}

func WithMaxAgentSteps(maxSteps int) Option {
	return func(engine *Engine) {
		if maxSteps > 0 {
			engine.maxAgentSteps = maxSteps
		}
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
	if len(e.tools) == 0 {
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

	fullResponse, err = e.runAgent(ctx, messages, onChunk)
	if err != nil {
		return nil, err
	}
	return &StreamResult{Content: fullResponse}, nil
}

func (e *Engine) runAgent(ctx context.Context, messages []domain.Message, onChunk func(string) error) (string, error) {
	log := logger.WithContext(e.log, ctx)
	lastUser := lastUserMessage(messages)
	log.Info("LLM Agent: Started",
		zap.String("provider", e.provider.Name()),
		zap.Int("max_steps", e.maxAgentSteps),
		zap.Strings("tools", toolNames(e.tools)),
		zap.Int("messages", len(messages)),
		zap.String("last_user_message_preview", preview(lastUser)),
	)

	modelProvider, ok := e.provider.(ModelProvider)
	if !ok {
		return "", fmt.Errorf("chat provider %q does not expose a LangChainGo model", e.provider.Name())
	}

	agent := agents.NewOneShotAgent(
		modelProvider.Model(),
		append([]Tool{}, e.tools...),
		agents.WithMaxIterations(e.maxAgentSteps),
		agents.WithPromptPrefix(agentPromptPrefix()),
	)
	executor := agents.NewExecutor(agent, agents.WithMaxIterations(e.maxAgentSteps))
	answer, err := chains.Run(ctx, executor, buildAgentInput(messages))
	if err != nil {
		if rawAnswer, ok := agentParseFallback(err); ok {
			log.Warn("LLM Agent: Using unformatted model output after parse failure",
				zap.Int("answer_bytes", len(rawAnswer)),
				zap.String("answer_preview", preview(rawAnswer)),
			)
			return emitFinal(rawAnswer, onChunk)
		}
		log.Error("LLM Agent: LangChainGo executor failed", zap.Error(err))
		return "", err
	}
	log.Info("LLM Agent: Completed", zap.Int("answer_bytes", len(answer)), zap.String("answer_preview", preview(answer)))
	return emitFinal(answer, onChunk)
}

func agentParseFallback(err error) (string, bool) {
	if !errors.Is(err, agents.ErrUnableToParseOutput) {
		return "", false
	}

	const parsePrefix = "unable to parse agent output:"
	raw := strings.TrimSpace(err.Error())
	if !strings.HasPrefix(raw, parsePrefix) {
		return "", false
	}

	raw = strings.TrimSpace(strings.TrimPrefix(raw, parsePrefix))
	return raw, raw != ""
}

func (e *Engine) callProvider(ctx context.Context, messages []domain.Message, onChunk func(string) error) (string, error) {
	var fullResponse string
	err := e.provider.Stream(ctx, messages, func(chunk string) error {
		fullResponse += chunk
		if onChunk == nil {
			return nil
		}
		return onChunk(chunk)
	})
	return fullResponse, err
}

func emitFinal(content string, onChunk func(string) error) (string, error) {
	if onChunk != nil {
		if err := onChunk(content); err != nil {
			return "", err
		}
	}
	return content, nil
}

func agentPromptPrefix() string {
	return `Answer the following questions as best you can. You have access to the following tools:

{{.tool_descriptions}}

Use web_search for current events, recent facts, live facts, weather, forecasts, or anything that may have changed recently.`
}

func buildAgentInput(messages []domain.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch msg.Role {
		case "system":
			b.WriteString("System: ")
		case "assistant":
			b.WriteString("Assistant: ")
		default:
			b.WriteString("User: ")
		}
		b.WriteString(content)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func toolNames(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	sort.Strings(names)
	return names
}

func preview(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= logPreviewLimit {
		return value
	}
	return value[:logPreviewLimit] + "...[truncated]"
}

func lastUserMessage(messages []domain.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

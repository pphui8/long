package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/pphui8/long/domain"
)

const defaultMaxAgentSteps = 3

func New(provider ChatProvider, opts ...Option) (*Engine, error) {
	if provider == nil {
		return nil, errors.New("chat provider is required")
	}

	engine := &Engine{
		contextBuilder: NewPassthroughContextBuilder(),
		maxAgentSteps:  defaultMaxAgentSteps,
		provider:       provider,
		promptBuilder:  NewBasicPromptBuilder(""),
		tools:          map[string]Tool{},
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

func WithTools(tools ...Tool) Option {
	return func(engine *Engine) {
		if engine.tools == nil {
			engine.tools = map[string]Tool{}
		}
		for _, tool := range tools {
			if tool == nil || strings.TrimSpace(tool.Name()) == "" {
				continue
			}
			engine.tools[tool.Name()] = tool
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

type agentDecision struct {
	Action      string `json:"action"`
	ActionInput string `json:"action_input"`
	Query       string `json:"query"`
	Final       string `json:"final"`
	FinalAnswer string `json:"final_answer"`
}

func (e *Engine) runAgent(ctx context.Context, messages []domain.Message, onChunk func(string) error) (string, error) {
	working := append([]domain.Message{}, messages...)
	working = appendAgentInstructions(working, e.tools)

	for step := 0; step < e.maxAgentSteps; step++ {
		raw, err := e.callProvider(ctx, working, nil)
		if err != nil {
			return "", err
		}

		decision, ok := parseAgentDecision(raw)
		if !ok {
			return emitFinal(raw, onChunk)
		}

		final := strings.TrimSpace(firstNonEmpty(decision.FinalAnswer, decision.Final))
		if final != "" {
			return emitFinal(final, onChunk)
		}

		action := strings.TrimSpace(decision.Action)
		if action == "" {
			return emitFinal(raw, onChunk)
		}

		tool, ok := e.tools[action]
		if !ok {
			working = append(working, domain.Message{
				Role:    "system",
				Content: fmt.Sprintf("Tool error: %q is not available. Available tools: %s.", action, strings.Join(toolNames(e.tools), ", ")),
			})
			continue
		}

		input := strings.TrimSpace(firstNonEmpty(decision.ActionInput, decision.Query))
		if input == "" {
			working = append(working, domain.Message{
				Role:    "system",
				Content: fmt.Sprintf("Tool error: %s requires a non-empty input.", action),
			})
			continue
		}

		result, err := tool.Execute(ctx, input)
		if err != nil {
			return "", fmt.Errorf("%s failed: %w", action, err)
		}

		working = append(working,
			domain.Message{Role: "assistant", Content: raw},
			domain.Message{Role: "system", Content: fmt.Sprintf("Tool result from %s for %q:\n%s", action, input, result)},
		)
	}

	working = append(working, domain.Message{
		Role:    "system",
		Content: "Agent step limit reached. Provide the final answer now using the conversation and tool results. Do not request another tool call.",
	})
	return e.callProvider(ctx, working, onChunk)
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

func appendAgentInstructions(messages []domain.Message, tools map[string]Tool) []domain.Message {
	var b strings.Builder
	b.WriteString("You are an agent. Decide whether a tool is needed before answering. ")
	b.WriteString("Use web_search for current events, recent facts, or questions that need outside web information. ")
	b.WriteString("When you need a tool, respond only with compact JSON like {\"action\":\"web_search\",\"action_input\":\"search query\"}. ")
	b.WriteString("When you can answer, respond only with compact JSON like {\"final_answer\":\"answer text\"}. ")
	b.WriteString("Available tools:\n")
	for _, name := range toolNames(tools) {
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(tools[name].Description())
		b.WriteByte('\n')
	}

	return append([]domain.Message{{Role: "system", Content: b.String()}}, messages...)
}

func parseAgentDecision(raw string) (agentDecision, bool) {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start < 0 || end < start {
		return agentDecision{}, false
	}
	text = text[start : end+1]

	var decision agentDecision
	if err := json.Unmarshal([]byte(text), &decision); err != nil {
		return agentDecision{}, false
	}
	return decision, true
}

func emitFinal(content string, onChunk func(string) error) (string, error) {
	if onChunk != nil {
		if err := onChunk(content); err != nil {
			return "", err
		}
	}
	return content, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func toolNames(tools map[string]Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

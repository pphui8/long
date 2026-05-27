package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

const defaultMaxAgentSteps = 3
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

func WithLogger(log *zap.Logger) Option {
	return func(engine *Engine) {
		if log != nil {
			engine.log = log
		}
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
	log := logger.WithContext(e.log, ctx)
	log.Info("LLM Agent: Started",
		zap.String("provider", e.provider.Name()),
		zap.Int("max_steps", e.maxAgentSteps),
		zap.Strings("tools", toolNames(e.tools)),
		zap.Int("messages", len(messages)),
		zap.String("last_user_message_preview", preview(lastUserMessage(messages))),
	)

	for step := 0; step < e.maxAgentSteps; step++ {
		stepNumber := step + 1
		callStart := time.Now()
		raw, err := e.callProvider(ctx, working, nil)
		if err != nil {
			log.Error("LLM Agent: Provider decision failed", zap.Int("step", stepNumber), zap.Duration("latency", time.Since(callStart)), zap.Error(err))
			return "", err
		}
		log.Debug("LLM Agent: Provider decision received",
			zap.Int("step", stepNumber),
			zap.Duration("latency", time.Since(callStart)),
			zap.Int("response_bytes", len(raw)),
			zap.String("response_preview", preview(raw)),
		)

		decision, ok := parseAgentDecision(raw)
		if !ok {
			log.Warn("LLM Agent: Provider returned non-JSON final response; no tool executed",
				zap.Int("step", stepNumber),
				zap.Int("response_bytes", len(raw)),
				zap.String("response_preview", preview(raw)),
			)
			return emitFinal(raw, onChunk)
		}

		final := strings.TrimSpace(firstNonEmpty(decision.FinalAnswer, decision.Final))
		if final != "" {
			log.Info("LLM Agent: Final answer selected",
				zap.Int("step", stepNumber),
				zap.Bool("tool_executed_this_step", false),
				zap.Int("answer_bytes", len(final)),
				zap.String("answer_preview", preview(final)),
			)
			return emitFinal(final, onChunk)
		}

		action := strings.TrimSpace(decision.Action)
		if action == "" {
			log.Warn("LLM Agent: JSON decision had no action or final answer",
				zap.Int("step", stepNumber),
				zap.String("decision_preview", preview(raw)),
			)
			return emitFinal(raw, onChunk)
		}

		tool, ok := e.tools[action]
		if !ok {
			log.Warn("LLM Agent: Requested unavailable tool",
				zap.Int("step", stepNumber),
				zap.String("tool", action),
				zap.Strings("available_tools", toolNames(e.tools)),
			)
			working = append(working, domain.Message{
				Role:    "system",
				Content: fmt.Sprintf("Tool error: %q is not available. Available tools: %s.", action, strings.Join(toolNames(e.tools), ", ")),
			})
			continue
		}

		input := strings.TrimSpace(firstNonEmpty(decision.ActionInput, decision.Query))
		if input == "" && !allowsEmptyInput(tool) {
			log.Warn("LLM Agent: Requested tool with empty input",
				zap.Int("step", stepNumber),
				zap.String("tool", action),
			)
			working = append(working, domain.Message{
				Role:    "system",
				Content: fmt.Sprintf("Tool error: %s requires a non-empty input.", action),
			})
			continue
		}

		log.Info("LLM Agent: Executing tool",
			zap.Int("step", stepNumber),
			zap.String("tool", action),
			zap.String("input", input),
		)
		toolStart := time.Now()
		result, err := tool.Execute(ctx, input)
		if err != nil {
			log.Error("LLM Agent: Tool failed",
				zap.Int("step", stepNumber),
				zap.String("tool", action),
				zap.String("input", input),
				zap.Duration("latency", time.Since(toolStart)),
				zap.Error(err),
			)
			return "", fmt.Errorf("%s failed: %w", action, err)
		}
		log.Info("LLM Agent: Tool completed",
			zap.Int("step", stepNumber),
			zap.String("tool", action),
			zap.String("input", input),
			zap.Duration("latency", time.Since(toolStart)),
			zap.Int("result_bytes", len(result)),
		)
		log.Debug("LLM Agent: Tool result",
			zap.Int("step", stepNumber),
			zap.String("tool", action),
			zap.String("input", input),
			zap.String("result_preview", preview(result)),
		)

		working = append(working,
			domain.Message{Role: "assistant", Content: raw},
			domain.Message{Role: "system", Content: fmt.Sprintf("Tool result from %s for %q:\n%s", action, input, result)},
		)
	}

	working = append(working, domain.Message{
		Role:    "system",
		Content: "Agent step limit reached. Provide the final answer now using the conversation and tool results. Do not request another tool call.",
	})
	log.Warn("LLM Agent: Step limit reached", zap.Int("max_steps", e.maxAgentSteps))
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
	b.WriteString("Use current_time for questions asking for the current date or time. ")
	b.WriteString("When you need a tool, respond only with compact JSON like {\"action\":\"web_search\",\"action_input\":\"search query\"} or {\"action\":\"current_time\",\"action_input\":\"UTC\"}. ")
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

func allowsEmptyInput(tool Tool) bool {
	emptyInputTool, ok := tool.(EmptyInputTool)
	return ok && emptyInputTool.AllowsEmptyInput()
}

func toolNames(tools map[string]Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
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

package llmengine

import (
	"context"
	"strings"
	"testing"

	"github.com/pphui8/long/domain"
	"github.com/tmc/langchaingo/llms"
)

type scriptedProvider struct {
	calls     [][]domain.Message
	responses []string
}

func (p *scriptedProvider) Name() string {
	return "test"
}

func (p *scriptedProvider) Stream(ctx context.Context, history []domain.Message, onChunk func(string) error) error {
	p.calls = append(p.calls, append([]domain.Message{}, history...))
	response := ""
	if len(p.responses) > 0 {
		response = p.responses[0]
		p.responses = p.responses[1:]
	}
	return onChunk(response)
}

type scriptedModelProvider struct {
	*scriptedProvider
	model *scriptedModel
}

func (p *scriptedModelProvider) Model() llms.Model {
	return p.model
}

type scriptedModel struct {
	calls     []string
	responses []string
}

func (m *scriptedModel) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	prompt := ""
	if len(messages) > 0 && len(messages[0].Parts) > 0 {
		if text, ok := messages[0].Parts[0].(llms.TextContent); ok {
			prompt = text.Text
		}
	}
	m.calls = append(m.calls, prompt)

	response := ""
	if len(m.responses) > 0 {
		response = m.responses[0]
		m.responses = m.responses[1:]
	}
	return &llms.ContentResponse{
		Choices: []*llms.ContentChoice{{Content: response}},
	}, nil
}

func (m *scriptedModel) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	m.calls = append(m.calls, prompt)
	response := ""
	if len(m.responses) > 0 {
		response = m.responses[0]
		m.responses = m.responses[1:]
	}
	return response, nil
}

type fakeTool struct {
	input string
}

func (t *fakeTool) Name() string {
	return "web_search"
}

func (t *fakeTool) Description() string {
	return "test search"
}

func (t *fakeTool) Call(ctx context.Context, input string) (string, error) {
	t.input = input
	return "source says current answer", nil
}

func TestEngineStreamsWithoutAgentWhenNoTools(t *testing.T) {
	provider := &scriptedProvider{responses: []string{"plain answer"}}
	engine, err := New(provider)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var streamed strings.Builder
	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "Say hi"}},
	}, func(chunk string) error {
		streamed.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if result.Content != "plain answer" {
		t.Fatalf("result = %q", result.Content)
	}
	if streamed.String() != result.Content {
		t.Fatalf("streamed = %q, result = %q", streamed.String(), result.Content)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.calls))
	}
}

func TestEngineAgentRunsLangChainGoToolAndReturnsFinalAnswer(t *testing.T) {
	model := &scriptedModel{responses: []string{
		"Action: web_search\nAction Input: latest release",
		"Final Answer: The current answer is grounded.",
	}}
	provider := &scriptedModelProvider{
		scriptedProvider: &scriptedProvider{},
		model:            model,
	}
	tool := &fakeTool{}
	engine, err := New(provider, WithTools(tool))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var streamed strings.Builder
	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "What is latest?"}},
	}, func(chunk string) error {
		streamed.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if tool.input != "latest release" {
		t.Fatalf("tool input = %q, want latest release", tool.input)
	}
	if result.Content != "The current answer is grounded." {
		t.Fatalf("result = %q", result.Content)
	}
	if streamed.String() != result.Content {
		t.Fatalf("streamed = %q, result = %q", streamed.String(), result.Content)
	}
	if len(model.calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(model.calls))
	}
	if !strings.Contains(model.calls[1], "Observation: source says current answer") {
		t.Fatalf("second prompt missing tool observation: %q", model.calls[1])
	}
}

func TestEngineAgentReturnsDirectFinalAnswer(t *testing.T) {
	model := &scriptedModel{responses: []string{"Final Answer: No search needed."}}
	provider := &scriptedModelProvider{
		scriptedProvider: &scriptedProvider{},
		model:            model,
	}
	engine, err := New(provider, WithTools(&fakeTool{}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "Say hi"}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if result.Content != "No search needed." {
		t.Fatalf("result = %q", result.Content)
	}
	if len(model.calls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.calls))
	}
}

func TestEngineAgentFallsBackToUnformattedDirectAnswer(t *testing.T) {
	model := &scriptedModel{responses: []string{"Kubernetes service discovery uses Services, DNS, and kube-proxy."}}
	provider := &scriptedModelProvider{
		scriptedProvider: &scriptedProvider{},
		model:            model,
	}
	engine, err := New(provider, WithTools(&fakeTool{}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	var streamed strings.Builder
	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "Explain K8s service discovery"}},
	}, func(chunk string) error {
		streamed.WriteString(chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	want := "Kubernetes service discovery uses Services, DNS, and kube-proxy."
	if result.Content != want {
		t.Fatalf("result = %q, want %q", result.Content, want)
	}
	if streamed.String() != want {
		t.Fatalf("streamed = %q, want %q", streamed.String(), want)
	}
	if len(model.calls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.calls))
	}
}

func TestEngineAgentRequiresLangChainGoModelProvider(t *testing.T) {
	engine, err := New(&scriptedProvider{}, WithTools(&fakeTool{}))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "What is latest?"}},
	}, nil)
	if err == nil {
		t.Fatal("Stream returned nil error, want model provider error")
	}
	if !strings.Contains(err.Error(), "does not expose a LangChainGo model") {
		t.Fatalf("error = %v", err)
	}
}

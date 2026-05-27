package llmengine

import (
	"context"
	"strings"
	"testing"

	"github.com/pphui8/long/domain"
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

type fakeTool struct {
	input string
}

func (t *fakeTool) Name() string {
	return "web_search"
}

func (t *fakeTool) Description() string {
	return "test search"
}

func (t *fakeTool) Execute(ctx context.Context, input string) (string, error) {
	t.input = input
	return "source says current answer", nil
}

func TestEngineAgentCallsToolAndReturnsFinalAnswer(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		`{"action":"web_search","action_input":"latest release"}`,
		`{"final_answer":"The current answer is grounded."}`,
	}}
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
	if len(provider.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.calls))
	}
	lastCall := provider.calls[1]
	if got := lastCall[len(lastCall)-1].Content; !strings.Contains(got, "source says current answer") {
		t.Fatalf("last tool result message = %q", got)
	}
}

func TestEngineAgentReturnsDirectFinalAnswer(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`{"final_answer":"No search needed."}`}}
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
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.calls))
	}
}

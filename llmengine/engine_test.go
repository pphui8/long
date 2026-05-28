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

type fakeEmptyInputTool struct {
	input string
}

func (t *fakeEmptyInputTool) Name() string {
	return "current_time"
}

func (t *fakeEmptyInputTool) Description() string {
	return "test time"
}

func (t *fakeEmptyInputTool) AllowsEmptyInput() bool {
	return true
}

func (t *fakeEmptyInputTool) Execute(ctx context.Context, input string) (string, error) {
	t.input = input
	return "Current time: 2026-05-27T12:00:00Z", nil
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

func TestEngineAgentAllowsEmptyInputTool(t *testing.T) {
	provider := &scriptedProvider{responses: []string{
		`{"action":"current_time"}`,
		`{"final_answer":"It is noon UTC."}`,
	}}
	tool := &fakeEmptyInputTool{}
	engine, err := New(provider, WithTools(tool))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "Use the clock tool."}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if tool.input != "" {
		t.Fatalf("tool input = %q, want empty", tool.input)
	}
	if result.Content != "It is noon UTC." {
		t.Fatalf("result = %q", result.Content)
	}
	if len(provider.calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.calls))
	}
}

func TestEngineAgentForcesCurrentTimeToolForTimeQuestion(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`It is 2026-05-27T21:00:00+09:00.`}}
	tool := &fakeEmptyInputTool{}
	engine, err := New(provider, WithTools(tool))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "What`s the time now?"}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if tool.input != "Asia/Tokyo" {
		t.Fatalf("tool input = %q, want Asia/Tokyo", tool.input)
	}
	if result.Content != "It is 2026-05-27T21:00:00+09:00." {
		t.Fatalf("result = %q", result.Content)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.calls))
	}
	lastCall := provider.calls[0]
	if got := lastCall[len(lastCall)-2].Content; !strings.Contains(got, "Required tool result from current_time") {
		t.Fatalf("required tool result message = %q", got)
	}
}

func TestEngineAgentForcesCurrentTimeToolForTimeInPlaceQuestion(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`It is 2026-05-28T22:56:00+09:00.`}}
	tool := &fakeEmptyInputTool{}
	engine, err := New(provider, WithTools(tool))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "What`s time in tokyo?"}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if tool.input != "Asia/Tokyo" {
		t.Fatalf("tool input = %q, want Asia/Tokyo", tool.input)
	}
	if result.Content != "It is 2026-05-28T22:56:00+09:00." {
		t.Fatalf("result = %q", result.Content)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.calls))
	}
	lastCall := provider.calls[0]
	if got := lastCall[len(lastCall)-2].Content; !strings.Contains(got, "Required tool result from current_time") {
		t.Fatalf("required tool result message = %q", got)
	}
}

func TestEngineAgentForcesTimeAndSearchForRelativeWeatherQuestion(t *testing.T) {
	provider := &scriptedProvider{responses: []string{`Tomorrow's forecast is grounded in search results.`}}
	timeTool := &fakeEmptyInputTool{}
	searchTool := &fakeTool{}
	engine, err := New(provider, WithTools(timeTool, searchTool))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	result, err := engine.Stream(context.Background(), StreamRequest{
		Username:       "user",
		ConversationID: 1,
		History:        []domain.Message{{Role: "user", Content: "What`s the weather tomorrow in Osaka, nakanoshima?"}},
	}, nil)
	if err != nil {
		t.Fatalf("Stream returned error: %v", err)
	}

	if timeTool.input != "Asia/Tokyo" {
		t.Fatalf("time tool input = %q, want Asia/Tokyo", timeTool.input)
	}
	if searchTool.input != "What`s the weather tomorrow in Osaka, nakanoshima?" {
		t.Fatalf("search tool input = %q, want original prompt", searchTool.input)
	}
	if result.Content != "Tomorrow's forecast is grounded in search results." {
		t.Fatalf("result = %q", result.Content)
	}
	if len(provider.calls) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.calls))
	}
	lastCall := provider.calls[0]
	var sawTime, sawSearch bool
	for _, msg := range lastCall {
		sawTime = sawTime || strings.Contains(msg.Content, "Required tool result from current_time")
		sawSearch = sawSearch || strings.Contains(msg.Content, "Required tool result from web_search")
	}
	if !sawTime || !sawSearch {
		t.Fatalf("provider call missing required tool results: sawTime=%v sawSearch=%v", sawTime, sawSearch)
	}
}

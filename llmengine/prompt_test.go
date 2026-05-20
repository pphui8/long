package llmengine

import (
	"testing"

	"github.com/pphui8/long/domain"
)

func TestBasicPromptBuilderPrependsSystemPrompt(t *testing.T) {
	builder := NewBasicPromptBuilder("")
	history := []domain.Message{
		{ConversationID: 7, Role: "user", Content: "Hello"},
	}

	messages, err := builder.Build(PromptInput{
		ConversationID: 7,
		Context:        &ModelContext{History: history},
	})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != "system" {
		t.Fatalf("expected first message role system, got %q", messages[0].Role)
	}
	if messages[0].Content != DefaultSystemPrompt {
		t.Fatalf("unexpected system prompt: %q", messages[0].Content)
	}
	if messages[1].Content != history[0].Content {
		t.Fatalf("expected history after system prompt, got %q", messages[1].Content)
	}
}

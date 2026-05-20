package llmengine

import "github.com/pphui8/long/domain"

const DefaultSystemPrompt = "you are a LLM to answer user's questions"

type PromptBuilder interface {
	Build(input PromptInput) ([]domain.Message, error)
}

type PromptInput struct {
	Username       string
	ConversationID int
	Context        *ModelContext
}

type BasicPromptBuilder struct {
	SystemPrompt string
}

func NewBasicPromptBuilder(systemPrompt string) *BasicPromptBuilder {
	if systemPrompt == "" {
		systemPrompt = DefaultSystemPrompt
	}
	return &BasicPromptBuilder{SystemPrompt: systemPrompt}
}

func (b *BasicPromptBuilder) Build(input PromptInput) ([]domain.Message, error) {
	history := []domain.Message{}
	if input.Context != nil {
		history = input.Context.History
	}

	messages := make([]domain.Message, 0, len(history)+1)
	if b.SystemPrompt != "" {
		messages = append(messages, domain.Message{
			ConversationID: input.ConversationID,
			Role:           "system",
			Content:        b.SystemPrompt,
		})
	}
	messages = append(messages, history...)
	return messages, nil
}

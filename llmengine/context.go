package llmengine

import (
	"context"
	"strconv"

	"github.com/pphui8/long/domain"
)

type ContextBuilder interface {
	Build(ctx context.Context, input ContextInput) (*ModelContext, error)
}

type ContextInput struct {
	Username       string
	ConversationID int
	History        []domain.Message
}

type ModelContext struct {
	History  []domain.Message
	Metadata map[string]string
}

type PassthroughContextBuilder struct{}

func NewPassthroughContextBuilder() *PassthroughContextBuilder {
	return &PassthroughContextBuilder{}
}

func (b *PassthroughContextBuilder) Build(ctx context.Context, input ContextInput) (*ModelContext, error) {
	return &ModelContext{
		History: input.History,
		Metadata: map[string]string{
			"username":        input.Username,
			"conversation_id": strconv.Itoa(input.ConversationID),
		},
	}, nil
}

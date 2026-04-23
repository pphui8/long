package repository

import (
	"context"
	"github.com/pphui8/long/domain"
)

type LLMRepository interface {
	SaveResult(ctx context.Context, res domain.LLMResponse) error
}

type llmRepository struct {
	// DB connection
}

func NewLLMRepository() LLMRepository {
	return &llmRepository{}
}

func (r *llmRepository) SaveResult(ctx context.Context, res domain.LLMResponse) error {
	// Placeholder for database operations
	return nil
}

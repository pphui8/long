package repository

import (
	"context"

	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
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
	logger.Log.Debug("DAO: Saving LLM result", zap.Any("response", res))
	// Placeholder for database operations
	return nil
}

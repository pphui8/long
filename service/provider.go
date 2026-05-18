package service

import (
	"context"

	"github.com/pphui8/long/domain"
)

type ChatProvider interface {
	Name() string
	Stream(ctx context.Context, history []domain.Message, onChunk func(string) error) error
}

type ProviderConfig struct {
	Name   string
	APIKey string
	Model  string
}

type ModelRegistry interface {
	DefaultModel(provider string) (string, bool)
}

type StaticModelRegistry map[string]string

func (r StaticModelRegistry) DefaultModel(provider string) (string, bool) {
	model, ok := r[provider]
	return model, ok
}

package handler

import (
	"database/sql"

	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/repository"
	"github.com/pphui8/long/service"
	"go.uber.org/zap"
)

type App struct {
	DB         *sql.DB
	TokenStore *auth.TokenStore
	UserRepo   repository.UserRepository
	LLMService service.LLMService
	Logger     *zap.Logger
}

package repository

import (
	"context"
	"database/sql"

	_ "github.com/lib/pq"
	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

type UserRepository interface {
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
}

type userRepository struct {
	db  *sql.DB
	log *zap.Logger
}

func NewUserRepository(db *sql.DB, log *zap.Logger) UserRepository {
	return &userRepository{db: db, log: log}
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	query := "SELECT username, password_hash FROM users WHERE username = $1"
	err := r.db.QueryRowContext(ctx, query, username).Scan(&user.Username, &user.PasswordHash)
	if err != nil {
		if err != sql.ErrNoRows {
			logger.WithContext(r.log, ctx).Error("DB: Failed to get user", zap.String("username", username), zap.Error(err))
		}
		return nil, err
	}
	return &user, nil
}

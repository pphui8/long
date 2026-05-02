package repository

import (
	"context"
	"database/sql"
	"github.com/pphui8/long/domain"
	_ "github.com/lib/pq"
)

type UserRepository interface {
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
}

type userRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	var user domain.User
	query := "SELECT username, password_hash, salt FROM users WHERE username = $1"
	err := r.db.QueryRowContext(ctx, query, username).Scan(&user.Username, &user.PasswordHash, &user.Salt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

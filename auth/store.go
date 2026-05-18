package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TokenStore implements refresh token rotation and revocation using Redis.
type TokenStore struct {
	client *redis.Client
	log    *zap.Logger
}

// InitRedis initializes a Redis-backed token store.
func InitRedis(addr, password string, db int, log *zap.Logger) (*TokenStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Check connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Error("Failed to connect to Redis", zap.String("addr", addr), zap.Error(err))
		_ = client.Close()
		return nil, err
	}

	log.Info("Connected to Redis", zap.String("addr", addr))
	return &TokenStore{client: client, log: log}, nil
}

func (s *TokenStore) RevokeToken(ctx context.Context, jti string) error {
	s.log.Debug("DAO: Revoking token", zap.String("jti", jti))
	err := s.client.Set(ctx, "revoked:"+jti, "1", RefreshTokenTTL).Err()
	if err != nil {
		s.log.Error("DAO Error: Failed to revoke token", zap.String("jti", jti), zap.Error(err))
	}
	return err
}

func (s *TokenStore) IsRevoked(ctx context.Context, jti string) bool {
	val, err := s.client.Exists(ctx, "revoked:"+jti).Result()
	if err != nil {
		s.log.Error("DAO Error: Failed to check if token is revoked", zap.String("jti", jti), zap.Error(err))
		return false
	}
	return val > 0
}

func (s *TokenStore) RegisterToken(ctx context.Context, username string, jti string) error {
	s.log.Debug("DAO: Registering token", zap.String("username", username), zap.String("jti", jti))
	err := s.client.Set(ctx, "active:"+username, jti, RefreshTokenTTL).Err()
	if err != nil {
		s.log.Error("DAO Error: Failed to register token", zap.String("username", username), zap.Error(err))
	}
	return err
}

func (s *TokenStore) InvalidateUserSessions(ctx context.Context, username string) error {
	s.log.Debug("DAO: Invalidating user sessions", zap.String("username", username))
	err := s.client.Del(ctx, "active:"+username).Err()
	if err != nil {
		s.log.Error("DAO Error: Failed to invalidate user sessions", zap.String("username", username), zap.Error(err))
	}
	return err
}

func (s *TokenStore) GetActiveToken(ctx context.Context, username string) string {
	val, err := s.client.Get(ctx, "active:"+username).Result()
	if err != nil && err != redis.Nil {
		s.log.Error("DAO Error: Failed to get active token", zap.String("username", username), zap.Error(err))
		return ""
	}
	return val
}

func (s *TokenStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

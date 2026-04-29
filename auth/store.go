package auth

import (
	"context"
	"time"

	"github.com/pphui8/long/logger"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TokenStore implements refresh token rotation and revocation using Redis.
type TokenStore struct {
	client *redis.Client
}

var GlobalTokenStore *TokenStore

// InitRedis initializes the GlobalTokenStore with a Redis client.
func InitRedis(addr, password string, db int) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	// Check connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		logger.Log.Error("Failed to connect to Redis", zap.String("addr", addr), zap.Error(err))
	} else {
		logger.Log.Info("Connected to Redis", zap.String("addr", addr))
	}

	GlobalTokenStore = &TokenStore{client: client}
}

func (s *TokenStore) RevokeToken(ctx context.Context, jti string) error {
	logger.Log.Debug("DAO: Revoking token", zap.String("jti", jti))
	err := s.client.Set(ctx, "revoked:"+jti, "1", 7*24*time.Hour).Err()
	if err != nil {
		logger.Log.Error("DAO Error: Failed to revoke token", zap.String("jti", jti), zap.Error(err))
	}
	return err
}

func (s *TokenStore) IsRevoked(ctx context.Context, jti string) bool {
	val, err := s.client.Exists(ctx, "revoked:"+jti).Result()
	if err != nil {
		logger.Log.Error("DAO Error: Failed to check if token is revoked", zap.String("jti", jti), zap.Error(err))
		return false
	}
	return val > 0
}

func (s *TokenStore) RegisterToken(ctx context.Context, username string, jti string) error {
	logger.Log.Debug("DAO: Registering token", zap.String("username", username), zap.String("jti", jti))
	err := s.client.Set(ctx, "active:"+username, jti, 7*24*time.Hour).Err()
	if err != nil {
		logger.Log.Error("DAO Error: Failed to register token", zap.String("username", username), zap.Error(err))
	}
	return err
}

func (s *TokenStore) InvalidateUserSessions(ctx context.Context, username string) error {
	logger.Log.Debug("DAO: Invalidating user sessions", zap.String("username", username))
	err := s.client.Del(ctx, "active:"+username).Err()
	if err != nil {
		logger.Log.Error("DAO Error: Failed to invalidate user sessions", zap.String("username", username), zap.Error(err))
	}
	return err
}

func (s *TokenStore) GetActiveToken(ctx context.Context, username string) string {
	val, err := s.client.Get(ctx, "active:"+username).Result()
	if err != nil && err != redis.Nil {
		logger.Log.Error("DAO Error: Failed to get active token", zap.String("username", username), zap.Error(err))
		return ""
	}
	return val
}

func (s *TokenStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

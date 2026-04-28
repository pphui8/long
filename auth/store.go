package auth

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
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
	GlobalTokenStore = &TokenStore{client: client}
}

func (s *TokenStore) RevokeToken(ctx context.Context, jti string) error {
	// Mark the token as revoked for 7 days (matching refresh token TTL)
	return s.client.Set(ctx, "revoked:"+jti, "1", 7*24*time.Hour).Err()
}

func (s *TokenStore) IsRevoked(ctx context.Context, jti string) bool {
	val, err := s.client.Exists(ctx, "revoked:"+jti).Result()
	if err != nil {
		return false
	}
	return val > 0
}

func (s *TokenStore) RegisterToken(ctx context.Context, username string, jti string) error {
	// Register the active token for the user
	return s.client.Set(ctx, "active:"+username, jti, 7*24*time.Hour).Err()
}

func (s *TokenStore) InvalidateUserSessions(ctx context.Context, username string) error {
	return s.client.Del(ctx, "active:"+username).Err()
}

func (s *TokenStore) GetActiveToken(ctx context.Context, username string) string {
	val, err := s.client.Get(ctx, "active:"+username).Result()
	if err != nil {
		return ""
	}
	return val
}

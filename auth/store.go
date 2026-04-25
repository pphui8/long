package auth

import (
	"sync"
)

// In a real application, this would be a Redis or Database store.
// For this example, we use an in-memory store.
type TokenStore struct {
	mu sync.RWMutex
	// maps refresh token ID to its status (true if revoked/used)
	revokedTokens map[string]bool
	// maps username to current active refresh token ID
	userActiveTokens map[string]string
}

var GlobalTokenStore = &TokenStore{
	revokedTokens:    make(map[string]bool),
	userActiveTokens: make(map[string]string),
}

func (s *TokenStore) RevokeToken(jti string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokedTokens[jti] = true
}

func (s *TokenStore) IsRevoked(jti string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.revokedTokens[jti]
}

func (s *TokenStore) RegisterToken(username string, jti string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userActiveTokens[username] = jti
}

func (s *TokenStore) InvalidateUserSessions(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// In a real app, we'd mark all tokens for this user as revoked.
	// Here we just clear the active token.
	delete(s.userActiveTokens, username)
}

func (s *TokenStore) GetActiveToken(username string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.userActiveTokens[username]
}

package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"

	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

var HashKey = func() []byte {
	key := os.Getenv("HASH_KEY")
	if key == "" {
		logger.Log.Error("Failed to load HASH_KEY from environment variables", zap.String("HASH_KEY", key))
		return []byte("default-hash-key")
	}
	return []byte(key)
}()

// HashPassword generates an HMAC-SHA256 hash of the password using the global HashKey.
func HashPassword(password string) (string, error) {
	h := hmac.New(sha256.New, HashKey)
	h.Write([]byte(password))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyPassword compares a password with a hash.
func VerifyPassword(password, hash string) (bool, error) {
	expectedHash, err := HashPassword(password)
	if err != nil {
		return false, err
	}
	return hmac.Equal([]byte(hash), []byte(expectedHash)), nil
}

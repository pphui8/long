package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"golang.org/x/crypto/argon2"
)

// Argon2 parameters
const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword generates an Argon2id hash of the password using a random salt.
func HashPassword(password string) (hash string, salt string, err error) {
	s := make([]byte, argonSaltLen)
	if _, err := rand.Read(s); err != nil {
		return "", "", err
	}

	h := argon2.IDKey([]byte(password), s, argonTime, argonMemory, argonThreads, argonKeyLen)

	return base64.RawStdEncoding.EncodeToString(h), base64.RawStdEncoding.EncodeToString(s), nil
}

// VerifyPassword compares a password with a hash and salt.
func VerifyPassword(password, hash, salt string) (bool, error) {
	h, err := base64.RawStdEncoding.DecodeString(hash)
	if err != nil {
		return false, err
	}

	s, err := base64.RawStdEncoding.DecodeString(salt)
	if err != nil {
		return false, err
	}

	comparisonHash := argon2.IDKey([]byte(password), s, argonTime, argonMemory, argonThreads, argonKeyLen)

	return subtle.ConstantTimeCompare(h, comparisonHash) == 1, nil
}

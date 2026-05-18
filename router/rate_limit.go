package router

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

const (
	geminiUserLimit     = 10
	geminiIPLimit       = 30
	geminiLimitWindow   = time.Minute
	geminiCleanupWindow = 5 * time.Minute
)

type rateLimitEntry struct {
	count       int
	windowStart time.Time
	lastSeen    time.Time
}

type fixedWindowLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitEntry
	limit   int
	window  time.Duration
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		entries: make(map[string]rateLimitEntry),
		limit:   limit,
		window:  window,
	}
}

func (l *fixedWindowLimiter) Allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.entries[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= l.window {
		entry = rateLimitEntry{windowStart: now}
	}

	entry.count++
	entry.lastSeen = now
	l.entries[key] = entry

	if entry.count <= l.limit {
		return true, 0
	}

	return false, l.window - now.Sub(entry.windowStart)
}

func (l *fixedWindowLimiter) Cleanup(now time.Time, maxAge time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, entry := range l.entries {
		if now.Sub(entry.lastSeen) > maxAge {
			delete(l.entries, key)
		}
	}
}

type abuseAPIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func GeminiAbuseProtection(log *zap.Logger) gin.HandlerFunc {
	userLimiter := newFixedWindowLimiter(geminiUserLimit, geminiLimitWindow)
	ipLimiter := newFixedWindowLimiter(geminiIPLimit, geminiLimitWindow)
	lastCleanup := time.Now()
	var cleanupMu sync.Mutex

	return func(c *gin.Context) {
		now := time.Now()
		cleanupMu.Lock()
		if now.Sub(lastCleanup) > geminiCleanupWindow {
			userLimiter.Cleanup(now, geminiCleanupWindow)
			ipLimiter.Cleanup(now, geminiCleanupWindow)
			lastCleanup = now
		}
		cleanupMu.Unlock()

		username, _ := c.Get("username")
		userKey := "anonymous"
		if value, ok := username.(string); ok && value != "" {
			userKey = value
		}

		ipKey := clientIPKey(c.ClientIP())
		if ok, retryAfter := userLimiter.Allow(userKey, now); !ok {
			abortRateLimited(c, log, retryAfter, "user", userKey)
			return
		}
		if ok, retryAfter := ipLimiter.Allow(ipKey, now); !ok {
			abortRateLimited(c, log, retryAfter, "ip", ipKey)
			return
		}

		c.Next()
	}
}

func clientIPKey(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	return parsed.String()
}

func abortRateLimited(c *gin.Context, fallback *zap.Logger, retryAfter time.Duration, scope string, key string) {
	retrySeconds := int(retryAfter.Seconds()) + 1
	if retrySeconds < 1 {
		retrySeconds = 1
	}

	c.Header("Retry-After", strconv.Itoa(retrySeconds))
	logger.FromGin(c, fallback).Warn("APP: Gemini rate limit exceeded", zap.String("scope", scope), zap.String("key", key), zap.Int("retry_after_seconds", retrySeconds))
	c.JSON(http.StatusTooManyRequests, gin.H{"error": abuseAPIError{
		Code:      "rate_limited",
		Message:   "Too many chat requests. Try again shortly.",
		RequestID: logger.RequestIDFromGin(c),
	}})
	c.Abort()
}

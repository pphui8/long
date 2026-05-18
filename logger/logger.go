package logger

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	RequestIDHeader = "X-Request-ID"
	RequestIDKey    = "request_id"
)

type requestIDContextKey struct{}

func Init(logPath string) *zap.Logger {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		panic(err)
	}

	lumberjackLogger := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    100,  // megabytes (backup limit, but daily rotation triggered by time)
		MaxBackups: 30,   // keep 30 days of files
		MaxAge:     30,   // days
		Compress:   true, // compress old logs
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, zapcore.AddSync(lumberjackLogger), zap.InfoLevel),
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), zap.InfoLevel),
	)

	return zap.New(core, zap.AddCaller())
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func RequestIDFromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if requestID, ok := c.Get(RequestIDKey); ok {
		if value, ok := requestID.(string); ok {
			return value
		}
	}
	return RequestIDFromContext(c.Request.Context())
}

func WithContext(log *zap.Logger, ctx context.Context) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}
	if requestID := RequestIDFromContext(ctx); requestID != "" {
		return log.With(zap.String(RequestIDKey, requestID))
	}
	return log
}

func FromGin(c *gin.Context, fallback *zap.Logger) *zap.Logger {
	if c == nil {
		return WithContext(fallback, nil)
	}
	return WithContext(fallback, c.Request.Context())
}

func Sync(log *zap.Logger) {
	if log != nil {
		_ = log.Sync()
	}
}

// GinLogger is a middleware that logs HTTP requests using Zap.
func GinLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(RequestIDKey, requestID)
		c.Writer.Header().Set(RequestIDHeader, requestID)
		c.Request = c.Request.WithContext(WithRequestID(c.Request.Context(), requestID))
		reqLog := WithContext(log, c.Request.Context())

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if len(c.Errors) > 0 {
			for _, e := range c.Errors.Errors() {
				reqLog.Error("Gin Error", zap.String("error", e))
			}
		} else {
			reqLog.Info("Access Log",
				zap.Int("status", status),
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.String("query", query),
				zap.String("ip", c.ClientIP()),
				zap.String("user-agent", c.Request.UserAgent()),
				zap.Duration("latency", latency),
			)
		}
	}
}

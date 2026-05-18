package logger

import (
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

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

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if len(c.Errors) > 0 {
			for _, e := range c.Errors.Errors() {
				log.Error("Gin Error", zap.String("error", e))
			}
		} else {
			log.Info("Access Log",
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

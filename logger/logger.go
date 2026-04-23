package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.Logger

func Init(logPath string) {
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

	Log = zap.New(core, zap.AddCaller())
}

func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}

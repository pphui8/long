package main

import (
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/router"
	"go.uber.org/zap"
)

func main() {
	logger.Init("log/logs/app.log")
	defer logger.Sync()

	logger.Log.Info("Starting Gin Web Server", zap.String("port", "9001"))

	r := router.Setup()

	if err := r.Run(":9001"); err != nil {
		logger.Log.Fatal("Failed to start server", zap.Error(err))
	}
}

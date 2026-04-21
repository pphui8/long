package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// Initialize logger with daily-rotating log file
	logger.Init("log/logs/app.log")
	defer logger.Sync()

	logger.Log.Info("Starting Gin Web Server", zap.String("port", "9000"))

	r := gin.Default()
	r.GET("/ping", func(c *gin.Context) {
		logger.Log.Info("Ping received", zap.String("client", c.ClientIP()))
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})
	// starts the business logic

	r.Run(":9001")
}

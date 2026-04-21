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

	logger.Log.Info("Starting Gin Web Server", zap.String("port", "9001"))

	r := gin.Default()

	// Since Nginx strips the "/api" prefix (e.g., llm.pphui8.com/api/ping -> localhost:9001/ping),
	// we use a router group that starts from the root to handle these requests.
	api := r.Group("/")
	{
		api.GET("/ping", func(c *gin.Context) {
			logger.Log.Info("Ping received", zap.String("client", c.ClientIP()))
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
			})
		})
		
		// Add future business logic routes here
	}

	r.Run(":9001")
}

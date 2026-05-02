package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

func HandlePing(c *gin.Context) {
	logger.Log.Info("Ping received", zap.String("client", c.ClientIP()))

	status := gin.H{
		"redis":    "up",
		"postgres": "up",
	}
	statusCode := http.StatusOK

	// Check Redis
	if auth.GlobalTokenStore == nil {
		status["redis"] = "not initialized"
		statusCode = http.StatusInternalServerError
	} else if err := auth.GlobalTokenStore.Ping(c); err != nil {
		logger.Log.Error("Redis ping failed", zap.Error(err))
		status["redis"] = "down"
		status["redis_error"] = err.Error()
		statusCode = http.StatusInternalServerError
	}

	// Check Postgres
	if auth.DB == nil {
		status["postgres"] = "not initialized"
		statusCode = http.StatusInternalServerError
	} else if err := auth.DB.Ping(); err != nil {
		logger.Log.Error("Postgres ping failed", zap.Error(err))
		status["postgres"] = "down"
		status["postgres_error"] = err.Error()
		statusCode = http.StatusInternalServerError
	}

	if statusCode == http.StatusOK {
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
			"status":  status,
		})
	} else {
		c.JSON(statusCode, gin.H{
			"message": "service unavailable",
			"status":  status,
		})
	}
}

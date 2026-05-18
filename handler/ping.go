package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (a *App) HandlePing(c *gin.Context) {
	a.Logger.Info("Ping received", zap.String("client", c.ClientIP()))

	status := gin.H{
		"redis":    "up",
		"postgres": "up",
	}
	statusCode := http.StatusOK

	// Check Redis
	if a.TokenStore == nil {
		status["redis"] = "not initialized"
		statusCode = http.StatusInternalServerError
	} else if err := a.TokenStore.Ping(c); err != nil {
		a.Logger.Error("Redis ping failed", zap.Error(err))
		status["redis"] = "down"
		status["redis_error"] = err.Error()
		statusCode = http.StatusInternalServerError
	}

	// Check Postgres
	if a.DB == nil {
		status["postgres"] = "not initialized"
		statusCode = http.StatusInternalServerError
	} else if err := a.DB.Ping(); err != nil {
		a.Logger.Error("Postgres ping failed", zap.Error(err))
		status["postgres"] = "down"
		status["postgres_error"] = err.Error()
		statusCode = http.StatusInternalServerError
	}

	if statusCode == http.StatusOK {
		respondData(c, http.StatusOK, gin.H{
			"message": "pong",
			"status":  status,
		})
	} else {
		respondData(c, statusCode, gin.H{
			"message": "service unavailable",
			"status":  status,
		})
	}
}

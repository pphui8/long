package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

func (a *App) HandlePing(c *gin.Context) {
	log := logger.FromGin(c, a.Logger)
	log.Info("Ping received", zap.String("client", c.ClientIP()))

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
		log.Error("Redis ping failed", zap.Error(err))
		status["redis"] = "down"
		statusCode = http.StatusInternalServerError
	}

	// Check Postgres
	if a.DB == nil {
		status["postgres"] = "not initialized"
		statusCode = http.StatusInternalServerError
	} else if err := a.DB.Ping(); err != nil {
		log.Error("Postgres ping failed", zap.Error(err))
		status["postgres"] = "down"
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

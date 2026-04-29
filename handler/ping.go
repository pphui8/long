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

	if auth.GlobalTokenStore == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "redis store not initialized",
		})
		return
	}

	err := auth.GlobalTokenStore.Ping(c)
	if err != nil {
		logger.Log.Error("Redis ping failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": "redis unavailable",
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}

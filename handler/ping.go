package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

func HandlePing(c *gin.Context) {
	logger.Log.Info("Ping received", zap.String("client", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}

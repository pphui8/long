package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/logger"
	"go.uber.org/zap"
)

func HandleResource(c *gin.Context) {
	username, _ := c.Get("username")
	logger.Log.Info("APP: Accessing protected resource", zap.Any("username", username))
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to the protected resource!",
		"user":    username,
	})
}

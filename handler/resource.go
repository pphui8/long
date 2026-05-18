package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (a *App) HandleResource(c *gin.Context) {
	username, _ := c.Get("username")
	a.Logger.Info("APP: Accessing protected resource", zap.Any("username", username))
	respondData(c, http.StatusOK, gin.H{
		"message": "Welcome to the protected resource!",
		"user":    username,
	})
}

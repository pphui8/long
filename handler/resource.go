package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func HandleResource(c *gin.Context) {
	username, _ := c.Get("username")
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to the protected resource!",
		"user":    username,
	})
}

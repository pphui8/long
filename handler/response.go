package handler

import "github.com/gin-gonic/gin"

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func respondData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func respondError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": apiError{
			Code:    code,
			Message: message,
		},
	})
}

package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/logger"
)

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func respondData(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data})
}

func respondError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{
		"error": apiError{
			Code:      code,
			Message:   message,
			RequestID: logger.RequestIDFromGin(c),
		},
	})
}

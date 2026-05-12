package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/db"
	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/repository"
	"github.com/pphui8/long/service"
	"go.uber.org/zap"
)

func HandleGemini(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req domain.LLMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Warn("APP: Gemini request binding failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	repo := repository.NewLLMRepository(db.Instance)
	llmSvc, err := service.NewLLMService(repo)
	if err != nil {
		logger.Log.Error("APP: Failed to initialize LLM service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize AI service"})
		return
	}

	resp, err := llmSvc.ProcessPrompt(c.Request.Context(), username.(string), req)
	if err != nil {
		logger.Log.Error("APP: Error processing Gemini prompt", zap.String("username", username.(string)), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

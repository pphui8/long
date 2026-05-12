package handler

import (
	"net/http"
	"strconv"

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

func HandleGetConversations(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	repo := repository.NewLLMRepository(db.Instance)
	llmSvc, err := service.NewLLMService(repo)
	if err != nil {
		logger.Log.Error("APP: Failed to initialize LLM service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize AI service"})
		return
	}

	conversations, err := llmSvc.GetConversations(c.Request.Context(), username.(string))
	if err != nil {
		logger.Log.Error("APP: Error fetching conversations", zap.String("username", username.(string)), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, conversations)
}

func HandleGetMessages(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	convIDStr := c.Param("id")
	if convIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Conversation ID is required"})
		return
	}

	convID, err := strconv.Atoi(convIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Conversation ID"})
		return
	}

	repo := repository.NewLLMRepository(db.Instance)
	llmSvc, err := service.NewLLMService(repo)
	if err != nil {
		logger.Log.Error("APP: Failed to initialize LLM service", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to initialize AI service"})
		return
	}

	messages, err := llmSvc.GetMessages(c.Request.Context(), username.(string), convID)
	if err != nil {
		logger.Log.Error("APP: Error fetching messages", zap.String("username", username.(string)), zap.Int("convID", convID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, messages)
}

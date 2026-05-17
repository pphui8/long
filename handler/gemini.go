package handler

import (
	"fmt"
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

	// Set headers for SSE
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Transfer-Encoding", "chunked")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	convID, err := llmSvc.StreamPrompt(c.Request.Context(), username.(string), req, func(chunk string) error {
		fmt.Fprintf(c.Writer, "data: %s\n\n", chunk)
		flusher.Flush()
		return nil
	})

	if err != nil {
		logger.Log.Error("APP: Error processing Gemini stream", zap.String("username", username.(string)), zap.Error(err))
		fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", err.Error())
		flusher.Flush()
		return
	}

	// Send the final conversation ID
	fmt.Fprintf(c.Writer, "event: done\ndata: {\"conversation_id\": %d}\n\n", convID)
	flusher.Flush()
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

func HandleDeleteConversation(c *gin.Context) {
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

	err = llmSvc.DeleteConversation(c.Request.Context(), username.(string), convID)
	if err != nil {
		logger.Log.Error("APP: Error deleting conversation", zap.String("username", username.(string)), zap.Int("convID", convID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

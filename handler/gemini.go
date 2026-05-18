package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/domain"
	"go.uber.org/zap"
)

func llmErrorStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func writeSSE(w io.Writer, event string, data string) error {
	if event != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
			return err
		}
	}

	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	for _, line := range strings.Split(data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return err
		}
	}

	_, err := io.WriteString(w, "\n")
	return err
}

func (a *App) HandleChat(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req domain.LLMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		a.Logger.Warn("APP: Chat request binding failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "prompt is required"})
		return
	}

	if req.ConversationID != nil {
		if err := a.LLMService.ValidateConversationAccess(c.Request.Context(), username.(string), *req.ConversationID); err != nil {
			a.Logger.Warn("APP: Chat conversation access denied", zap.String("username", username.(string)), zap.Int("convID", *req.ConversationID), zap.Error(err))
			c.JSON(llmErrorStatus(err), gin.H{"error": err.Error()})
			return
		}
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

	convID, err := a.LLMService.StreamPrompt(c.Request.Context(), username.(string), req, func(chunk string) error {
		if err := writeSSE(c.Writer, "", chunk); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})

	if err != nil {
		a.Logger.Error("APP: Error processing chat stream", zap.String("username", username.(string)), zap.Error(err))
		_ = writeSSE(c.Writer, "error", err.Error())
		flusher.Flush()
		return
	}

	// Send the final conversation ID
	_ = writeSSE(c.Writer, "done", fmt.Sprintf("{\"conversation_id\": %d}", convID))
	flusher.Flush()
}

func (a *App) HandleGetConversations(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	conversations, err := a.LLMService.GetConversations(c.Request.Context(), username.(string))
	if err != nil {
		a.Logger.Error("APP: Error fetching conversations", zap.String("username", username.(string)), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, conversations)
}

func (a *App) HandleGetMessages(c *gin.Context) {
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

	messages, err := a.LLMService.GetMessages(c.Request.Context(), username.(string), convID)
	if err != nil {
		a.Logger.Error("APP: Error fetching messages", zap.String("username", username.(string)), zap.Int("convID", convID), zap.Error(err))
		c.JSON(llmErrorStatus(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, messages)
}

func (a *App) HandleDeleteConversation(c *gin.Context) {
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

	err = a.LLMService.DeleteConversation(c.Request.Context(), username.(string), convID)
	if err != nil {
		a.Logger.Error("APP: Error deleting conversation", zap.String("username", username.(string)), zap.Int("convID", convID), zap.Error(err))
		c.JSON(llmErrorStatus(err), gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

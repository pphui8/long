package handler

import (
	"encoding/json"
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

const (
	maxChatRequestBodyBytes = 64 * 1024
	maxPromptRunes          = 8000
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

func llmErrorCode(err error) string {
	switch {
	case errors.Is(err, domain.ErrValidation):
		return "invalid_request"
	case errors.Is(err, domain.ErrForbidden):
		return "forbidden"
	case errors.Is(err, domain.ErrNotFound):
		return "not_found"
	default:
		return "internal_error"
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
		respondError(c, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	var req domain.LLMRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		a.Logger.Warn("APP: Chat request binding failed", zap.Error(err))
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			respondError(c, http.StatusRequestEntityTooLarge, "request_too_large", fmt.Sprintf("request body must be at most %d bytes", maxChatRequestBodyBytes))
			return
		}
		respondError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		respondError(c, http.StatusBadRequest, "invalid_prompt", "prompt is required")
		return
	}
	if len([]rune(req.Prompt)) > maxPromptRunes {
		respondError(c, http.StatusBadRequest, "prompt_too_large", fmt.Sprintf("prompt must be at most %d characters", maxPromptRunes))
		return
	}

	if req.ConversationID != nil {
		if err := a.LLMService.ValidateConversationAccess(c.Request.Context(), username.(string), *req.ConversationID); err != nil {
			a.Logger.Warn("APP: Chat conversation access denied", zap.String("username", username.(string)), zap.Int("convID", *req.ConversationID), zap.Error(err))
			respondError(c, llmErrorStatus(err), llmErrorCode(err), err.Error())
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
		respondError(c, http.StatusInternalServerError, "streaming_unsupported", "Streaming not supported")
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
		_ = writeSSEJSON(c.Writer, "error", gin.H{"error": apiError{Code: llmErrorCode(err), Message: err.Error()}})
		flusher.Flush()
		return
	}

	// Send the final conversation ID
	_ = writeSSEJSON(c.Writer, "done", gin.H{"data": gin.H{"conversation_id": convID}})
	flusher.Flush()
}

func (a *App) HandleGetConversations(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		respondError(c, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	conversations, err := a.LLMService.GetConversations(c.Request.Context(), username.(string))
	if err != nil {
		a.Logger.Error("APP: Error fetching conversations", zap.String("username", username.(string)), zap.Error(err))
		respondError(c, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	respondData(c, http.StatusOK, conversations)
}

func (a *App) HandleGetMessages(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		respondError(c, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	convIDStr := c.Param("id")
	if convIDStr == "" {
		respondError(c, http.StatusBadRequest, "invalid_request", "Conversation ID is required")
		return
	}

	convID, err := strconv.Atoi(convIDStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request", "Invalid Conversation ID")
		return
	}

	messages, err := a.LLMService.GetMessages(c.Request.Context(), username.(string), convID)
	if err != nil {
		a.Logger.Error("APP: Error fetching messages", zap.String("username", username.(string)), zap.Int("convID", convID), zap.Error(err))
		respondError(c, llmErrorStatus(err), llmErrorCode(err), err.Error())
		return
	}

	respondData(c, http.StatusOK, messages)
}

func (a *App) HandleDeleteConversation(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		respondError(c, http.StatusUnauthorized, "unauthorized", "Unauthorized")
		return
	}

	convIDStr := c.Param("id")
	if convIDStr == "" {
		respondError(c, http.StatusBadRequest, "invalid_request", "Conversation ID is required")
		return
	}

	convID, err := strconv.Atoi(convIDStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid_request", "Invalid Conversation ID")
		return
	}

	err = a.LLMService.DeleteConversation(c.Request.Context(), username.(string), convID)
	if err != nil {
		a.Logger.Error("APP: Error deleting conversation", zap.String("username", username.(string)), zap.Int("convID", convID), zap.Error(err))
		respondError(c, llmErrorStatus(err), llmErrorCode(err), err.Error())
		return
	}

	respondData(c, http.StatusOK, gin.H{"message": "Conversation deleted successfully"})
}

func writeSSEJSON(w io.Writer, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSE(w, event, string(data))
}

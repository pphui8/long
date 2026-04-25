package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
)

func TestHandleRefreshRotation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/refresh", HandleRefresh)

	// 1. Generate an initial refresh token
	username := "admin"
	refreshToken, err := auth.GenerateRefreshToken(username)
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	// 2. Perform refresh request
	reqBody, _ := json.Marshal(map[string]string{
		"refresh_token": refreshToken,
	})
	req, _ := http.NewRequest(http.MethodPost, "/refresh", bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// 3. Verify response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %v", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	newAccessToken := resp["access_token"]
	newRefreshToken := resp["refresh_token"]

	if newAccessToken == "" {
		t.Error("Expected new access token, got empty string")
	}
	if newRefreshToken == "" {
		t.Error("Expected new refresh token (rotation), got empty string")
	}
	if newRefreshToken == refreshToken {
		t.Error("Expected refresh token to be rotated, but it's the same")
	}
}

package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/domain"
)

func HandleLogin(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Username == "admin" && req.Password == "password123" {
		accessToken, err := auth.GenerateAccessToken(req.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
			return
		}

		refreshToken, jti, err := auth.GenerateRefreshToken(req.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
			return
		}

		// Register the token in our store for rotation tracking
		auth.GlobalTokenStore.RegisterToken(c, req.Username, jti)

		// Set Refresh Token in HttpOnly cookie
		// Determine if we should set Secure flag based on request
		secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("refresh_token", refreshToken, 7*24*3600, "/", "", secure, true)

		c.JSON(http.StatusOK, gin.H{
			"access_token": accessToken,
		})
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
	}
}

func HandleRefresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token cookie missing"})
		return
	}

	claims, err := auth.ValidateToken(refreshToken, auth.RefreshAudience)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	// Refresh Token Rotation Logic
	// 1. Check if this token was already revoked (reused)
	if auth.GlobalTokenStore.IsRevoked(c, claims.ID) {
		// Potential reuse attack! Invalidate all sessions for this user.
		auth.GlobalTokenStore.InvalidateUserSessions(c, claims.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token reuse detected. All sessions invalidated."})
		return
	}

	// 2. Check if this is still the active token for the user
	activeJti := auth.GlobalTokenStore.GetActiveToken(c, claims.Username)
	if activeJti != claims.ID {
		// This token is not the latest one, but it wasn't explicitly revoked.
		// In a rotation scenario, this should also be treated as a risk if it's not the active one.
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token is no longer active"})
		return
	}

	// 3. Mark the current token as used (revoked)
	auth.GlobalTokenStore.RevokeToken(c, claims.ID)

	// 4. Generate new pair
	newAccessToken, err := auth.GenerateAccessToken(claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	newRefreshToken, newJti, err := auth.GenerateRefreshToken(claims.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// 5. Register the new token
	auth.GlobalTokenStore.RegisterToken(c, claims.Username, newJti)

	// 6. Set the new Refresh Token cookie
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", newRefreshToken, 7*24*3600, "/", "", secure, true)

	c.JSON(http.StatusOK, gin.H{
		"access_token": newAccessToken,
	})
}

package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/domain"
	"github.com/pphui8/long/logger"
	"github.com/pphui8/long/repository"
	"go.uber.org/zap"
)

func HandleLogin(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Log.Warn("APP: Login request binding failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logger.Log.Info("APP: Processing login request", zap.String("username", req.Username))

	userRepo := repository.NewUserRepository(auth.DB)
	user, err := userRepo.GetByUsername(c, req.Username)

	if err == nil {
		match, err := auth.VerifyPassword(req.Password, user.PasswordHash)
		if err != nil {
			logger.Log.Error("APP: Error verifying password", zap.String("username", req.Username), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		if match {
			accessToken, err := auth.GenerateAccessToken(req.Username)
			if err != nil {
				logger.Log.Error("APP: Failed to generate access token", zap.String("username", req.Username), zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
				return
			}

			refreshToken, jti, err := auth.GenerateRefreshToken(req.Username)
			if err != nil {
				logger.Log.Error("APP: Failed to generate refresh token", zap.String("username", req.Username), zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
				return
			}

			// Register the token in our store for rotation tracking
			if err := auth.GlobalTokenStore.RegisterToken(c, req.Username, jti); err != nil {
				// Even if registration fails, we might still want to proceed, or fail fast.
				// Let's log it.
				logger.Log.Error("APP: Failed to register token in store", zap.String("username", req.Username), zap.Error(err))
			}

			// Set Refresh Token in HttpOnly cookie
			secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie("refresh_token", refreshToken, 7*24*3600, "/", "", secure, true)

			logger.Log.Info("APP: Login successful", zap.String("username", req.Username))
			c.JSON(http.StatusOK, gin.H{
				"access_token": accessToken,
			})
			return
		}
	} else if err != sql.ErrNoRows {
		logger.Log.Error("APP: Database error during login", zap.String("username", req.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	logger.Log.Warn("APP: Invalid login attempt", zap.String("username", req.Username))
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
}

func HandleRefresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		logger.Log.Warn("APP: Refresh token cookie missing")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token cookie missing"})
		return
	}

	claims, err := auth.ValidateToken(refreshToken, auth.RefreshAudience)
	if err != nil {
		logger.Log.Warn("APP: Invalid refresh token", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	logger.Log.Info("APP: Processing refresh token", zap.String("username", claims.Username), zap.String("jti", claims.ID))

	// Refresh Token Rotation Logic
	// 1. Check if this token was already revoked (reused)
	if auth.GlobalTokenStore.IsRevoked(c, claims.ID) {
		logger.Log.Warn("APP: Token reuse detected!", zap.String("username", claims.Username), zap.String("jti", claims.ID))
		// Potential reuse attack! Invalidate all sessions for this user.
		auth.GlobalTokenStore.InvalidateUserSessions(c, claims.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token reuse detected. All sessions invalidated."})
		return
	}

	// 2. Check if this is still the active token for the user
	activeJti := auth.GlobalTokenStore.GetActiveToken(c, claims.Username)
	if activeJti != claims.ID {
		logger.Log.Warn("APP: Token is no longer active", zap.String("username", claims.Username), zap.String("jti", claims.ID), zap.String("activeJti", activeJti))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token is no longer active"})
		return
	}

	// 3. Mark the current token as used (revoked)
	auth.GlobalTokenStore.RevokeToken(c, claims.ID)

	// 4. Generate new pair
	newAccessToken, err := auth.GenerateAccessToken(claims.Username)
	if err != nil {
		logger.Log.Error("APP: Failed to generate access token during refresh", zap.String("username", claims.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	newRefreshToken, newJti, err := auth.GenerateRefreshToken(claims.Username)
	if err != nil {
		logger.Log.Error("APP: Failed to generate refresh token during refresh", zap.String("username", claims.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// 5. Register the new token
	auth.GlobalTokenStore.RegisterToken(c, claims.Username, newJti)

	// 6. Set the new Refresh Token cookie
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", newRefreshToken, 7*24*3600, "/", "", secure, true)

	logger.Log.Info("APP: Token refresh successful", zap.String("username", claims.Username))
	c.JSON(http.StatusOK, gin.H{
		"access_token": newAccessToken,
	})
}

package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/domain"
	"go.uber.org/zap"
)

func (a *App) HandleLogin(c *gin.Context) {
	var req domain.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		a.Logger.Warn("APP: Login request binding failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	a.Logger.Info("APP: Processing login request", zap.String("username", req.Username))

	user, err := a.UserRepo.GetByUsername(c, req.Username)

	if err == nil {
		match, err := auth.VerifyPassword(req.Password, user.PasswordHash)
		if err != nil {
			a.Logger.Error("APP: Error verifying password", zap.String("username", req.Username), zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
			return
		}

		if match {
			accessToken, err := auth.GenerateAccessToken(req.Username)
			if err != nil {
				a.Logger.Error("APP: Failed to generate access token", zap.String("username", req.Username), zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
				return
			}

			refreshToken, jti, err := auth.GenerateRefreshToken(req.Username)
			if err != nil {
				a.Logger.Error("APP: Failed to generate refresh token", zap.String("username", req.Username), zap.Error(err))
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
				return
			}

			// Register the token in our store for rotation tracking
			if err := a.TokenStore.RegisterToken(c, req.Username, jti); err != nil {
				a.Logger.Error("APP: Failed to register token in store", zap.String("username", req.Username), zap.Error(err))
			}

			// Set Refresh Token in HttpOnly cookie
			secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
			c.SetSameSite(http.SameSiteLaxMode)
			c.SetCookie("refresh_token", refreshToken, int(auth.RefreshTokenTTL.Seconds()), "/", "", secure, true)

			a.Logger.Info("APP: Login successful", zap.String("username", req.Username))
			c.JSON(http.StatusOK, gin.H{
				"access_token": accessToken,
			})
			return
		}
	} else if err != sql.ErrNoRows {
		a.Logger.Error("APP: Database error during login", zap.String("username", req.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal server error"})
		return
	}

	a.Logger.Warn("APP: Invalid login attempt", zap.String("username", req.Username))
	c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
}

func (a *App) HandleRefresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil {
		a.Logger.Warn("APP: Refresh token cookie missing")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Refresh token cookie missing"})
		return
	}

	claims, err := auth.ValidateToken(refreshToken, auth.RefreshAudience)
	if err != nil {
		a.Logger.Warn("APP: Invalid refresh token", zap.Error(err))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
		return
	}

	a.Logger.Info("APP: Processing refresh token", zap.String("username", claims.Username), zap.String("jti", claims.ID))

	// Refresh Token Rotation Logic
	// 1. Check if this token was already revoked (reused)
	if a.TokenStore.IsRevoked(c, claims.ID) {
		a.Logger.Warn("APP: Token reuse detected!", zap.String("username", claims.Username), zap.String("jti", claims.ID))
		// Potential reuse attack! Invalidate all sessions for this user.
		a.TokenStore.InvalidateUserSessions(c, claims.Username)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token reuse detected. All sessions invalidated."})
		return
	}

	// 2. Check if this is still the active token for the user
	activeJti := a.TokenStore.GetActiveToken(c, claims.Username)
	if activeJti != claims.ID {
		a.Logger.Warn("APP: Token is no longer active", zap.String("username", claims.Username), zap.String("jti", claims.ID), zap.String("activeJti", activeJti))
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token is no longer active"})
		return
	}

	// 3. Mark the current token as used (revoked)
	a.TokenStore.RevokeToken(c, claims.ID)

	// 4. Generate new pair
	newAccessToken, err := auth.GenerateAccessToken(claims.Username)
	if err != nil {
		a.Logger.Error("APP: Failed to generate access token during refresh", zap.String("username", claims.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
		return
	}

	newRefreshToken, newJti, err := auth.GenerateRefreshToken(claims.Username)
	if err != nil {
		a.Logger.Error("APP: Failed to generate refresh token during refresh", zap.String("username", claims.Username), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
		return
	}

	// 5. Register the new token
	a.TokenStore.RegisterToken(c, claims.Username, newJti)

	// 6. Set the new Refresh Token cookie
	secure := c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("refresh_token", newRefreshToken, int(auth.RefreshTokenTTL.Seconds()), "/", "", secure, true)

	a.Logger.Info("APP: Token refresh successful", zap.String("username", claims.Username))
	c.JSON(http.StatusOK, gin.H{
		"access_token": newAccessToken,
	})
}

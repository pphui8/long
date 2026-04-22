package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/pkg/auth"
	"github.com/pphui8/long/pkg/logger"
	"go.uber.org/zap"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func main() {
	// Initialize logger with daily-rotating log file
	logger.Init("log/logs/app.log")
	defer logger.Sync()

	logger.Log.Info("Starting Gin Web Server", zap.String("port", "9001"))

	r := gin.Default()

	r.POST("/login", func(c *gin.Context) {
		var req LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Fixed username and password
		if req.Username == "admin" && req.Password == "password123" {
			accessToken, err := auth.GenerateAccessToken(req.Username)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
				return
			}

			refreshToken, err := auth.GenerateRefreshToken(req.Username)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate refresh token"})
				return
			}

			c.JSON(http.StatusOK, gin.H{
				"access_token":  accessToken,
				"refresh_token": refreshToken,
			})
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		}
	})

	r.POST("/refresh", func(c *gin.Context) {
		var req RefreshRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		claims, err := auth.ValidateToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid refresh token"})
			return
		}

		newAccessToken, err := auth.GenerateAccessToken(claims.Username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate access token"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"access_token": newAccessToken,
		})
	})

	api := r.Group("/")
	{
		api.GET("/ping", func(c *gin.Context) {
			logger.Log.Info("Ping received", zap.String("client", c.ClientIP()))
			c.JSON(http.StatusOK, gin.H{
				"message": "pong",
			})
		})
	}

	// Protected routes
	protected := r.Group("/api")
	protected.Use(auth.AuthMiddleware())
	{
		protected.GET("/resource", func(c *gin.Context) {
			username, _ := c.Get("username")
			c.JSON(http.StatusOK, gin.H{
				"message": "Welcome to the protected resource!",
				"user":    username,
			})
		})
	}

	r.Run(":9001")
}

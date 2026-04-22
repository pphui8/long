package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/pkg/auth"
	"github.com/pphui8/long/pkg/logger"
	"go.uber.org/zap"
)

// Request structures
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

	r := setupRouter()

	if err := r.Run(":9001"); err != nil {
		logger.Log.Fatal("Failed to start server", zap.Error(err))
	}
}

func setupRouter() *gin.Engine {
	r := gin.Default()

	// Since Nginx strips the /api prefix, all routes here are relative to that prefix.
	// For example, llm.pphui8.com/api/login -> localhost:9001/login

	// Public routes
	r.POST("/login", handleLogin)
	r.POST("/refresh", handleRefresh)
	r.GET("/ping", handlePing)

	// Protected routes
	protected := r.Group("/")
	protected.Use(auth.AuthMiddleware())
	{
		protected.GET("/resource", handleResource)
	}

	return r
}

// Handler functions
func handleLogin(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Fixed username and password for now
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
}

func handleRefresh(c *gin.Context) {
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
}

func handlePing(c *gin.Context) {
	logger.Log.Info("Ping received", zap.String("client", c.ClientIP()))
	c.JSON(http.StatusOK, gin.H{
		"message": "pong",
	})
}

func handleResource(c *gin.Context) {
	username, _ := c.Get("username")
	c.JSON(http.StatusOK, gin.H{
		"message": "Welcome to the protected resource!",
		"user":    username,
	})
}

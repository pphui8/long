package router

import (
	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/handler"
	"github.com/pphui8/long/logger"
)

func Setup() *gin.Engine {
	r := gin.New() // Use New() to avoid default middleware
	r.Use(logger.GinLogger(), gin.Recovery())

	// CORS middleware
	r.Use(CORSMiddleware())

	// Public routes
	r.POST("/login", handler.HandleLogin)
	r.POST("/refresh", handler.HandleRefresh)

	r.GET("/ping", handler.HandlePing)

	// Protected routes
	protected := r.Group("/")
	protected.Use(auth.AuthMiddleware())
	{
		protected.GET("/resource", handler.HandleResource)
	}

	return r
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "https://llm.pphui8.com")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

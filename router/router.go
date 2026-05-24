package router

import (
	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/auth"
	"github.com/pphui8/long/handler"
	"github.com/pphui8/long/logger"
)

func Setup(app *handler.App) *gin.Engine {
	r := gin.New()
	r.Use(logger.GinLogger(app.Logger), gin.Recovery())

	// CORS middleware
	r.Use(CORSMiddleware())

	// Public routes
	r.POST("/login", app.HandleLogin)
	r.POST("/refresh", app.HandleRefresh)

	r.GET("/ping", app.HandlePing)

	// Protected routes
	protected := r.Group("/")
	protected.Use(auth.AuthMiddleware())
	chatAbuseProtection := ChatAbuseProtection(app.Logger)
	{
		protected.GET("/resource", app.HandleResource)
		protected.POST("/post", chatAbuseProtection, app.HandleChat)
		protected.POST("/gemini", chatAbuseProtection, app.HandleChat)
		protected.GET("/conversations", app.HandleGetConversations)
		protected.GET("/conversations/:id/messages", app.HandleGetMessages)
		protected.GET("/conversations/:id/delete", app.HandleDeleteConversation)
	}

	return r
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "https://llm.pphui8.com")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-Request-ID")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

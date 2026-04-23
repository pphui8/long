package router

import (
	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/handler"
	"github.com/pphui8/long/auth"
)

func Setup() *gin.Engine {
	r := gin.Default()

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

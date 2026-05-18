package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pphui8/long/logger"
)

type authAPIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func abortWithAuthError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, gin.H{"error": authAPIError{
		Code:      code,
		Message:   message,
		RequestID: logger.RequestIDFromGin(c),
	}})
	c.Abort()
}

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			abortWithAuthError(c, http.StatusUnauthorized, "authorization_missing", "Authorization header is required")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			abortWithAuthError(c, http.StatusUnauthorized, "authorization_invalid", "Authorization header must be Bearer token")
			return
		}

		claims, err := ValidateToken(parts[1], AccessAudience)
		if err != nil {
			abortWithAuthError(c, http.StatusUnauthorized, "access_token_invalid", "Invalid or expired access token")
			return
		}

		c.Set("username", claims.Username)
		c.Next()
	}
}

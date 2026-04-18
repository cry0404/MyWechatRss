package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	SessionCookieName = "session"
	bearerPrefix      = "Bearer "
)

const CurrentUserIDKey = "current_user_id"

func RequireUser(signer *Signer) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "login required"})
			return
		}
		claims, err := signer.Verify(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.Set(CurrentUserIDKey, claims.UID)
		c.Next()
	}
}

func CurrentUserID(c *gin.Context) int64 {
	if v, ok := c.Get(CurrentUserIDKey); ok {
		if id, ok := v.(int64); ok {
			return id
		}
	}
	return 0
}

func extractToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	if strings.HasPrefix(h, bearerPrefix) {
		return strings.TrimPrefix(h, bearerPrefix)
	}
	if ck, err := c.Cookie(SessionCookieName); err == nil && ck != "" {
		return ck
	}
	return ""
}

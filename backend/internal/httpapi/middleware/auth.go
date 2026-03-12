package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"onlineqacommunity/backend/internal/config"
	"onlineqacommunity/backend/internal/service/auth"
)

// ClaimsContextKey 是请求上下文中保存鉴权声明的键名。
const ClaimsContextKey = "auth_claims"

/**
 * RequireAuth 校验访问令牌有效性，并拒绝已撤销的令牌继续访问受保护接口。
 */
func RequireAuth(authSvc *auth.Service, cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			c.Abort()
			return
		}

		tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		claims, err := auth.ParseAndValidateToken(tokenString, cfg)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}
		if claims.TokenType != auth.TokenTypeAccess {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token type"})
			c.Abort()
			return
		}

		revoked, err := authSvc.IsTokenRevoked(c.Request.Context(), claims.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "auth check failed"})
			c.Abort()
			return
		}
		if revoked {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "token revoked"})
			c.Abort()
			return
		}

		c.Set(ClaimsContextKey, claims)
		c.Next()
	}
}

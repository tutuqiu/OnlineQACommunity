package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"onlineqacommunity/backend/internal/config"
	authmodule "onlineqacommunity/backend/internal/httpapi/modules/auth"
)

/**
 * NewRouter 创建并注册 HTTP 路由，包括认证模块与健康检查接口。
 */
func NewRouter(db *sql.DB, cfg config.Config) *gin.Engine {
	r := gin.Default()

	authmodule.RegisterRoutes(r, db, cfg)

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "onlineqa backend is running",
			"env":     cfg.AppEnv,
		})
	})

	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		dbStatus := "ok"
		if err := db.PingContext(ctx); err != nil {
			dbStatus = "down"
		}

		c.JSON(http.StatusOK, gin.H{
			"app":    cfg.AppName,
			"status": "ok",
			"db":     dbStatus,
			"port":   cfg.AppPort,
		})
	})

	return r
}

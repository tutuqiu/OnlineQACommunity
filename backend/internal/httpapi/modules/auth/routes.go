package authmodule

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"onlineqacommunity/backend/internal/config"
	"onlineqacommunity/backend/internal/httpapi/middleware"
	"onlineqacommunity/backend/internal/model"
	authsvc "onlineqacommunity/backend/internal/service/auth"
)

const requestTimeout = 3 * time.Second

type authHandler struct {
	svc *authsvc.Service
}

func RegisterRoutes(r *gin.Engine, db *sql.DB, cfg config.Config) {
	svc := authsvc.New(db, cfg)
	h := &authHandler{svc: svc}

	group := r.Group("/api/v1/auth")
	group.POST("/register", h.register)
	group.POST("/login", h.login)
	group.POST("/refresh", h.refresh)

	protected := group.Group("")
	protected.Use(middleware.RequireAuth(svc, cfg))
	protected.POST("/logout", h.logout)
}

func (h *authHandler) register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	user, err := h.svc.Register(ctx, req)
	if err != nil {
		if errors.Is(err, authsvc.ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "email or username already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := model.RegisterResponse{User: user}
	c.JSON(http.StatusCreated, resp)
}

func (h *authHandler) login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	user, token, err := h.svc.Login(ctx, req)
	if err != nil {
		if errors.Is(err, authsvc.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}

	resp := model.LoginResponse{
		AccessToken:      token.AccessToken,
		RefreshToken:     token.RefreshToken,
		TokenType:        token.TokenType,
		ExpiresIn:        token.ExpiresIn,
		RefreshExpiresIn: token.RefreshExpiresIn,
		User:             user,
	}
	c.JSON(http.StatusOK, resp)
}

func (h *authHandler) refresh(c *gin.Context) {
	var req model.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	token, err := h.svc.RefreshToken(ctx, req.RefreshToken)
	if err != nil {
		if errors.Is(err, authsvc.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "refresh token failed"})
		return
	}

	resp := model.RefreshTokenResponse{
		AccessToken:      token.AccessToken,
		RefreshToken:     token.RefreshToken,
		TokenType:        token.TokenType,
		ExpiresIn:        token.ExpiresIn,
		RefreshExpiresIn: token.RefreshExpiresIn,
	}
	c.JSON(http.StatusOK, resp)
}

func (h *authHandler) logout(c *gin.Context) {
	rawClaims, exists := c.Get(middleware.ClaimsContextKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing token claims"})
		return
	}

	claims, ok := rawClaims.(*authsvc.CustomClaims)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	if err := h.svc.RevokeToken(ctx, claims); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "logout failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "logout success"})
}

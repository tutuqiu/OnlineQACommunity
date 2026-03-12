package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"onlineqacommunity/backend/internal/config"
	"onlineqacommunity/backend/internal/model"
)

var (
	// ErrUserExists 表示邮箱或用户名已存在。
	ErrUserExists = errors.New("user already exists")
	// ErrInvalidCredentials 表示认证凭证校验失败。
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrWeakPassword 表示注册密码未满足强密码规则。
	ErrWeakPassword = errors.New("weak password")
)

const (
	// TokenTypeAccess 表示访问令牌类型。
	TokenTypeAccess = "Bearer"
	// TokenTypeRefresh 表示刷新令牌类型。
	TokenTypeRefresh = "Refresh"
)

// CustomClaims 定义 JWT 中扩展的令牌类型声明。
type CustomClaims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

// Service 封装认证相关的数据库访问与令牌逻辑。
type Service struct {
	db  *sql.DB
	cfg config.Config
}

/**
 * New 创建认证服务实例。
 */
func New(db *sql.DB, cfg config.Config) *Service {
	return &Service{db: db, cfg: cfg}
}

/**
 * Register 创建新用户并返回基础用户信息。
 */
func (s *Service) Register(ctx context.Context, req model.RegisterRequest) (model.User, error) {
	if !isStrongPassword(req.Password) {
		return model.User{}, ErrWeakPassword
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, fmt.Errorf("hash password: %w", err)
	}

	var user model.User
	q := `
INSERT INTO users (email, username, password_hash)
VALUES ($1, $2, $3)
RETURNING id, email, username;
`
	err = s.db.QueryRowContext(ctx, q, strings.ToLower(req.Email), req.Username, string(passwordHash)).Scan(
		&user.ID, &user.Email, &user.Username,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return model.User{}, ErrUserExists
		}
		return model.User{}, fmt.Errorf("insert user: %w", err)
	}

	return user, nil
}

/**
 * Login 校验用户凭证并签发新的令牌对。
 */
func (s *Service) Login(ctx context.Context, req model.LoginRequest) (model.User, model.AuthToken, error) {
	user, passwordHash, err := s.getUserByEmail(ctx, strings.ToLower(req.Email))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, model.AuthToken{}, ErrInvalidCredentials
		}
		return model.User{}, model.AuthToken{}, fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return model.User{}, model.AuthToken{}, ErrInvalidCredentials
	}

	tokenModel, err := s.issueTokenPair(user.ID)
	if err != nil {
		return model.User{}, model.AuthToken{}, fmt.Errorf("issue token pair: %w", err)
	}

	return user, tokenModel, nil
}

/**
 * RefreshToken 校验刷新令牌、撤销旧令牌并签发新令牌对。
 */
func (s *Service) RefreshToken(ctx context.Context, rawRefreshToken string) (model.AuthToken, error) {
	claims, err := ParseAndValidateToken(rawRefreshToken, s.cfg)
	if err != nil {
		return model.AuthToken{}, ErrInvalidCredentials
	}
	if claims.TokenType != TokenTypeRefresh {
		return model.AuthToken{}, ErrInvalidCredentials
	}

	revoked, err := s.IsTokenRevoked(ctx, claims.ID)
	if err != nil {
		return model.AuthToken{}, fmt.Errorf("check refresh token revoked: %w", err)
	}
	if revoked {
		return model.AuthToken{}, ErrInvalidCredentials
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return model.AuthToken{}, ErrInvalidCredentials
	}

	if _, _, err := s.getUserByID(ctx, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.AuthToken{}, ErrInvalidCredentials
		}
		return model.AuthToken{}, fmt.Errorf("query user: %w", err)
	}

	if err := s.RevokeToken(ctx, claims); err != nil {
		return model.AuthToken{}, fmt.Errorf("revoke old refresh token: %w", err)
	}

	tokenModel, err := s.issueTokenPair(userID)
	if err != nil {
		return model.AuthToken{}, fmt.Errorf("issue token pair: %w", err)
	}
	return tokenModel, nil
}

/**
 * RevokeToken 将指定令牌加入撤销表，后续请求将无法继续使用。
 */
func (s *Service) RevokeToken(ctx context.Context, claims *CustomClaims) error {
	if claims == nil || claims.ExpiresAt == nil || claims.ID == "" {
		return nil
	}

	q := `
INSERT INTO revoked_tokens (jti, expires_at)
VALUES ($1, $2)
ON CONFLICT (jti) DO NOTHING;
`
	_, err := s.db.ExecContext(ctx, q, claims.ID, claims.ExpiresAt.Time)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

/**
 * IsTokenRevoked 检查令牌是否已在撤销表中且仍处于有效期内。
 */
func (s *Service) IsTokenRevoked(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}

	q := `
SELECT EXISTS (
  SELECT 1 FROM revoked_tokens
  WHERE jti = $1 AND expires_at > NOW()
);
`
	var exists bool
	if err := s.db.QueryRowContext(ctx, q, jti).Scan(&exists); err != nil {
		return false, fmt.Errorf("check revoked token: %w", err)
	}
	return exists, nil
}

/**
 * ParseAndValidateToken 解析 JWT 并校验签名算法、签发方与过期时间。
 */
func ParseAndValidateToken(tokenString string, cfg config.Config) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(
		tokenString,
		&CustomClaims{},
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, fmt.Errorf("unexpected signing method: %s", token.Method.Alg())
			}
			return []byte(cfg.JWTSecret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(cfg.JWTIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

/**
 * getUserByEmail 按邮箱查询用户和密码哈希。
 */
func (s *Service) getUserByEmail(ctx context.Context, email string) (model.User, string, error) {
	var (
		user         model.User
		passwordHash string
	)

	q := `
SELECT id, email, username, password_hash
FROM users
WHERE email = $1;
`
	err := s.db.QueryRowContext(ctx, q, email).Scan(&user.ID, &user.Email, &user.Username, &passwordHash)
	if err != nil {
		return model.User{}, "", err
	}
	return user, passwordHash, nil
}

/**
 * getUserByID 按用户 ID 查询用户和密码哈希。
 */
func (s *Service) getUserByID(ctx context.Context, userID int64) (model.User, string, error) {
	var (
		user         model.User
		passwordHash string
	)

	q := `
SELECT id, email, username, password_hash
FROM users
WHERE id = $1;
`
	err := s.db.QueryRowContext(ctx, q, userID).Scan(&user.ID, &user.Email, &user.Username, &passwordHash)
	if err != nil {
		return model.User{}, "", err
	}
	return user, passwordHash, nil
}

/**
 * issueTokenPair 生成访问令牌与刷新令牌，并计算各自剩余有效期。
 */
func (s *Service) issueTokenPair(userID int64) (model.AuthToken, error) {
	now := time.Now()
	accessExp := now.Add(time.Duration(s.cfg.JWTAccessTTLMinutes) * time.Minute)
	refreshExp := now.Add(time.Duration(s.cfg.JWTRefreshTTLHours) * time.Hour)

	accessToken, err := s.signToken(userID, TokenTypeAccess, now, accessExp)
	if err != nil {
		return model.AuthToken{}, fmt.Errorf("sign access token: %w", err)
	}
	refreshToken, err := s.signToken(userID, TokenTypeRefresh, now, refreshExp)
	if err != nil {
		return model.AuthToken{}, fmt.Errorf("sign refresh token: %w", err)
	}

	return model.AuthToken{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        TokenTypeAccess,
		ExpiresIn:        int64(time.Until(accessExp).Seconds()),
		RefreshExpiresIn: int64(time.Until(refreshExp).Seconds()),
	}, nil
}

/**
 * signToken 根据用户标识和令牌类型签名生成 JWT 字符串。
 */
func (s *Service) signToken(userID int64, tokenType string, now, exp time.Time) (string, error) {
	claims := &CustomClaims{
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			Issuer:    s.cfg.JWTIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        newTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

/**
 * newTokenID 生成随机令牌唯一标识。
 */
func newTokenID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

/**
 * isStrongPassword 校验密码是否满足强密码规则：长度不少于 8，且字符类型至少命中两类。
 */
func isStrongPassword(password string) bool {
	if len(password) < 8 {
		return false
	}

	var hasUpper bool
	var hasLower bool
	var hasDigit bool
	var hasSpecial bool

	for _, r := range password {
		switch {
		case r >= 'A' && r <= 'Z':
			hasUpper = true
		case r >= 'a' && r <= 'z':
			hasLower = true
		case r >= '0' && r <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	categoryCount := 0
	if hasUpper {
		categoryCount++
	}
	if hasLower {
		categoryCount++
	}
	if hasDigit {
		categoryCount++
	}
	if hasSpecial {
		categoryCount++
	}

	return categoryCount >= 2
}

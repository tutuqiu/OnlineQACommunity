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
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

const (
	TokenTypeAccess  = "Bearer"
	TokenTypeRefresh = "Refresh"
)

type CustomClaims struct {
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

type Service struct {
	db  *sql.DB
	cfg config.Config
}

func New(db *sql.DB, cfg config.Config) *Service {
	return &Service{db: db, cfg: cfg}
}

func (s *Service) Register(ctx context.Context, req model.RegisterRequest) (model.User, error) {
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

func newTokenID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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

type CustomClaims struct {
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
	var (
		userID       int64
		email        string
		username     string
		passwordHash string
	)

	q := `
SELECT id, email, username, password_hash
FROM users
WHERE email = $1;
`
	err := s.db.QueryRowContext(ctx, q, strings.ToLower(req.Email)).Scan(&userID, &email, &username, &passwordHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, model.AuthToken{}, ErrInvalidCredentials
		}
		return model.User{}, model.AuthToken{}, fmt.Errorf("query user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return model.User{}, model.AuthToken{}, ErrInvalidCredentials
	}

	now := time.Now()
	exp := now.Add(time.Duration(s.cfg.JWTAccessTTLMinutes) * time.Minute)

	claims := &CustomClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			Issuer:    s.cfg.JWTIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        newTokenID(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	accessToken, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return model.User{}, model.AuthToken{}, fmt.Errorf("sign token: %w", err)
	}

	user := model.User{
		ID:       userID,
		Email:    email,
		Username: username,
	}
	tokenModel := model.AuthToken{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int64(time.Until(exp).Seconds()),
	}
	return user, tokenModel, nil
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

func newTokenID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

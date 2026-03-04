package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppName string
	AppEnv  string
	AppHost string
	AppPort string

	DatabaseURL      string
	DBMaxOpenConns   int
	DBMaxIdleConns   int
	DBConnMaxLifeSec int
	DBConnMaxIdleSec int

	JWTSecret           string
	JWTIssuer           string
	JWTAccessTTLMinutes int
}

func FromEnv() Config {
	return Config{
		AppName:             getEnv("APP_NAME", "onlineqa-backend"),
		AppEnv:              getEnv("APP_ENV", "dev"),
		AppHost:             getEnv("APP_HOST", "0.0.0.0"),
		AppPort:             getEnv("APP_PORT", "18765"),
		DatabaseURL:         mustEnv("DATABASE_URL"),
		DBMaxOpenConns:      getEnvInt("DB_MAX_OPEN_CONNS", 30),
		DBMaxIdleConns:      getEnvInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifeSec:    getEnvInt("DB_CONN_MAX_LIFETIME_SEC", 1800),
		DBConnMaxIdleSec:    getEnvInt("DB_CONN_MAX_IDLE_TIME_SEC", 300),
		JWTSecret:           mustEnv("JWT_SECRET"),
		JWTIssuer:           getEnv("JWT_ISSUER", "onlineqa-backend"),
		JWTAccessTTLMinutes: getEnvInt("JWT_ACCESS_TTL_MINUTES", 120),
	}
}

func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%s", c.AppHost, c.AppPort)
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required env: " + key)
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

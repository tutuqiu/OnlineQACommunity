package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config 聚合应用启动所需的环境变量配置。
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
	JWTRefreshTTLHours  int
}

/**
 * FromEnv 从环境变量读取配置，并在缺省时回退到约定默认值。
 */
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
		JWTRefreshTTLHours:  getEnvInt("JWT_REFRESH_TTL_HOURS", 720),
	}
}

/**
 * ListenAddr 返回 HTTP 服务监听地址。
 */
func (c Config) ListenAddr() string {
	return fmt.Sprintf("%s:%s", c.AppHost, c.AppPort)
}

/**
 * getEnv 读取可选环境变量，缺失时返回默认值。
 */
func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

/**
 * mustEnv 读取必填环境变量，缺失时直接中断启动。
 */
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic("missing required env: " + key)
	}
	return v
}

/**
 * getEnvInt 读取整型环境变量，解析失败时回退到默认值。
 */
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

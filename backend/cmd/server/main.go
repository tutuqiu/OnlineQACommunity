package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"onlineqacommunity/backend/internal/config"
	"onlineqacommunity/backend/internal/httpapi"
)

func main() {
	cfg := config.FromEnv()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database failed: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.DBConnMaxLifeSec) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(cfg.DBConnMaxIdleSec) * time.Second)

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("database ping failed: %v", err)
	}

	router := httpapi.NewRouter(db, cfg)
	if err := router.Run(cfg.ListenAddr()); err != nil {
		log.Fatalf("server start failed: %v", err)
	}
}
